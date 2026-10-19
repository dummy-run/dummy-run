package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Roles, in the order the README lists them.
const (
	Viewer   = "viewer"
	Author   = "author"
	Editor   = "editor"
	Reviewer = "reviewer"
	Admin    = "admin"
)

// TestPassword is the password of every seeded user. All data in Inkwell is fake.
const TestPassword = "inkwell-test"

type User struct {
	ID, Name, Email, Role string
}

type Draft struct {
	ID         int
	Title      string
	Body       string
	OwnerID    string
	Status     string // draft, in_review, approved, rejected, published
	ReviewerID string
	Words      int
}

type Review struct {
	DraftID    int
	ReviewerID string
	Verdict    string
}

type Issue struct {
	ID    int
	Title string
	Date  time.Time
	Slots int
}

type Payment struct {
	ID       int
	DraftID  int
	AuthorID string
	Pence    int
	Status   string // pending, approved
}

type session struct {
	userID  string
	created time.Time
}

// Server holds all state in memory. A restart or /_reset returns to the seed data.
type Server struct {
	mu        sync.Mutex
	sw        map[string]bool
	inject    bool
	testLogin bool
	now       func() time.Time
	sleep     func(time.Duration)

	users    []*User
	sessions map[string]*session
	drafts   []*Draft
	reviews  []*Review
	issues   []*Issue
	payments []*Payment
	fails    map[string][]time.Time
	siteName string
	nextID   int
}

func NewServer(m *Manifest, testLogin bool) *Server {
	s := &Server{
		sw:        m.Enabled(),
		inject:    m != nil && m.Injection,
		testLogin: testLogin,
		now:       time.Now,
		sleep:     time.Sleep,
	}
	s.seed()
	return s
}

func (s *Server) seed() {
	s.users = []*User{
		{"vik", "Vik Viewer", "vik@inkwell.test", Viewer},
		{"sam", "Sam Author", "sam@inkwell.test", Author},
		{"lin", "Lin Author", "lin@inkwell.test", Author},
		{"ada", "Ada Editor", "ada@inkwell.test", Editor},
		{"ravi", "Ravi Reviewer", "ravi@inkwell.test", Reviewer},
		{"root", "Rae Admin", "root@inkwell.test", Admin},
	}
	s.sessions = map[string]*session{}
	s.fails = map[string][]time.Time{}
	s.siteName = "Inkwell"
	s.drafts = []*Draft{
		{1, "Spring reading list", "Twelve books for the first warm weeks.", "sam", "draft", "", 640},
		{2, "The harbour essay", "A long walk along the quay in winter.", "lin", "in_review", "", 2100},
		{3, "Winter notes", "Short pieces written by candlelight.", "sam", "approved", "ravi", 980},
		{4, "A guide to letterpress", "How a small press sets type.", "lin", "published", "ravi", 1500},
		{5, "Editor's letter", "A few words from the desk.", "ada", "approved", "ravi", 420},
		{6, "Archive: the 1998 floods", "What the paper printed that week.", "sam", "draft", "", 1800},
		{7, "Notes on proofreading", "Marks that every writer should know.", "lin", "published", "ravi", 760},
	}
	s.reviews = []*Review{
		{3, "ravi", "approve"},
		{4, "ravi", "approve"},
		{5, "ravi", "approve"},
		{7, "ravi", "approve"},
	}
	base := s.now().UTC().Truncate(24 * time.Hour)
	s.issues = []*Issue{
		{1, "Autumn issue", base.AddDate(0, 0, 14), 3},
		{2, "Winter issue", base.AddDate(0, 0, 60), 5},
	}
	s.payments = []*Payment{
		{1, 4, "lin", 12000, "approved"},
		{2, 7, "lin", 8000, "approved"},
		{3, 3, "sam", 9000, "pending"},
		{4, 5, "ada", 4500, "pending"},
	}
	s.nextID = 100
}

func (s *Server) has(k string) bool { return s.sw[k] }

func (s *Server) userByID(id string) *User {
	for _, u := range s.users {
		if u.ID == id {
			return u
		}
	}
	return nil
}

func (s *Server) draftByID(raw string) *Draft {
	id, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	for _, d := range s.drafts {
		if d.ID == id {
			return d
		}
	}
	return nil
}

func (s *Server) sessionTTL() time.Duration {
	if s.has("fric-session-short") {
		return 2 * time.Minute
	}
	return 12 * time.Hour
}

func (s *Server) current(r *http.Request) *User {
	c, err := r.Cookie("inkwell_session")
	if err != nil {
		return nil
	}
	sess := s.sessions[c.Value]
	if sess == nil {
		return nil
	}
	if s.now().Sub(sess.created) > s.sessionTTL() {
		delete(s.sessions, c.Value)
		return nil
	}
	return s.userByID(sess.userID)
}

func (s *Server) startSession(w http.ResponseWriter, u *User) {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	tok := hex.EncodeToString(b)
	s.sessions[tok] = &session{userID: u.ID, created: s.now()}
	http.SetCookie(w, &http.Cookie{Name: "inkwell_session", Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode}) //nolint:gosec // Inkwell is served over plain HTTP on localhost, so Secure would drop the cookie // nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure
}

func in(role string, roles ...string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

// Policy. Each function states the documented rule, then the planted exceptions.

func (s *Server) canEdit(u *User, d *Draft) bool {
	if u == nil || d.Status == "published" {
		return false
	}
	if s.has("perm-viewer-edit") && u.Role == Viewer {
		return true
	}
	if s.has("perm-author-edit-others") && u.Role == Author {
		return true
	}
	switch u.Role {
	case Author, Editor:
		return d.OwnerID == u.ID
	case Admin:
		return true
	}
	return false
}

func (s *Server) canSubmit(u *User, d *Draft) bool {
	return u != nil && d.OwnerID == u.ID && (d.Status == "draft" || d.Status == "rejected") && in(u.Role, Author, Editor)
}

func (s *Server) canReview(u *User, d *Draft) bool {
	if u == nil || d.Status != "in_review" {
		return false
	}
	if d.OwnerID == u.ID {
		return s.has("perm-owner-self-review") && u.Role == Author
	}
	return in(u.Role, Reviewer, Admin)
}

func (s *Server) canPublish(u *User, d *Draft) bool {
	if u == nil || d.Status != "approved" || !in(u.Role, Editor, Admin) {
		return false
	}
	if d.OwnerID == u.ID {
		return s.has("perm-editor-self-publish") && u.Role == Editor
	}
	return true
}

func (s *Server) canDelete(u *User) bool {
	return u != nil && (u.Role == Admin || (s.has("perm-editor-delete") && u.Role == Editor))
}

func (s *Server) canSeeReviews(u *User) bool {
	if u == nil {
		return false
	}
	if u.Role == Reviewer {
		return !s.has("perm-reviewer-denied")
	}
	return in(u.Role, Editor, Admin)
}

func (s *Server) canSeePayments(u *User) bool {
	if u == nil {
		return false
	}
	if u.Role == Reviewer && s.has("perm-reviewer-payments") {
		return true
	}
	return in(u.Role, Author, Editor, Admin)
}

// Page is what every template receives.
type Page struct {
	Title  string
	User   *User
	Flash  string
	Path   string
	Data   any
	Inject bool
	sw     map[string]bool
	Site   string
}

func (p Page) Has(k string) bool { return p.sw[k] }

func (s *Server) page(r *http.Request, u *User, title string, data any) Page {
	return Page{Title: title, User: u, Path: r.URL.Path, Data: data, Inject: s.inject, sw: s.sw, Site: s.siteName,
		Flash: r.URL.Query().Get("msg")}
}

func (s *Server) render(w http.ResponseWriter, status int, name string, p Page) {
	t, ok := templates[name]
	if !ok {
		http.Error(w, "no template "+name, http.StatusInternalServerError)
		return
	}
	var sb strings.Builder
	if err := t.ExecuteTemplate(&sb, "layout", p); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(sb.String())) // nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter // html/template output
}

func (s *Server) forbidden(w http.ResponseWriter, r *http.Request, u *User, what string) {
	s.render(w, http.StatusForbidden, "forbidden", s.page(r, u, "Not allowed", what))
}

func (s *Server) needSignIn(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/signin?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther) //nolint:gosec // fixed local path, the query is escaped // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
}

func redirectMsg(w http.ResponseWriter, r *http.Request, to, msg string) {
	sep := "?"
	if strings.Contains(to, "?") {
		sep = "&"
	}
	http.Redirect(w, r, to+sep+"msg="+url.QueryEscape(msg), http.StatusSeeOther)
}

// guard wraps a handler with the state lock and, when asked, a sign-in check.
func (s *Server) guard(needUser bool, h func(w http.ResponseWriter, r *http.Request, u *User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/reviews" && s.has("fric-reviews-slow") {
			s.sleep(3 * time.Second)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		u := s.current(r)
		if needUser && u == nil {
			s.needSignIn(w, r)
			return
		}
		h(w, r, u)
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintln(w, "ok") })
	mux.HandleFunc("GET /{$}", s.guard(false, s.home))
	mux.HandleFunc("GET /help", s.guard(false, s.help))
	mux.HandleFunc("GET /cover.svg", coverHandler)
	mux.HandleFunc("GET /signin", s.guard(false, s.signinForm))
	mux.HandleFunc("POST /signin", s.guard(false, s.signinPost))
	mux.HandleFunc("POST /signout", s.guard(false, s.signout))
	mux.HandleFunc("GET /drafts", s.guard(true, s.draftList))
	mux.HandleFunc("GET /drafts/new", s.guard(true, s.draftNewForm))
	mux.HandleFunc("POST /drafts/new", s.guard(true, s.draftNewPost))
	mux.HandleFunc("GET /drafts/{id}", s.guard(true, s.draftShow))
	mux.HandleFunc("GET /drafts/{id}/edit", s.guard(true, s.draftEditForm))
	mux.HandleFunc("POST /drafts/{id}/edit", s.guard(true, s.draftEditPost))
	mux.HandleFunc("POST /drafts/{id}/submit", s.guard(true, s.draftSubmit))
	mux.HandleFunc("POST /drafts/{id}/review", s.guard(true, s.draftReview))
	mux.HandleFunc("POST /drafts/{id}/publish", s.guard(true, s.draftPublish))
	mux.HandleFunc("POST /drafts/{id}/delete", s.guard(true, s.draftDelete))
	mux.HandleFunc("GET /reviews", s.guard(true, s.reviewList))
	mux.HandleFunc("GET /issues", s.guard(true, s.issueList))
	mux.HandleFunc("POST /issues", s.guard(true, s.issueCreate))
	mux.HandleFunc("GET /payments", s.guard(true, s.paymentList))
	mux.HandleFunc("POST /payments/{id}/approve", s.guard(true, s.paymentApprove))
	mux.HandleFunc("GET /admin/users", s.guard(true, s.userList))
	mux.HandleFunc("POST /admin/users/{id}/role", s.guard(true, s.userRole))
	mux.HandleFunc("GET /settings", s.guard(true, s.settingsForm))
	mux.HandleFunc("POST /settings", s.guard(true, s.settingsPost))
	if s.testLogin {
		mux.HandleFunc("GET /test-login", s.guard(false, s.testLoginHandler))
		mux.HandleFunc("POST /_reset", s.reset)
	}
	return mux
}

var _ = template.HTML("")
