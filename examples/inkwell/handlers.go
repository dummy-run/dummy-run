package main

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *Server) home(w http.ResponseWriter, r *http.Request, u *User) {
	s.render(w, 200, "home", s.page(r, u, "Home", nil))
}

func (s *Server) help(w http.ResponseWriter, r *http.Request, u *User) {
	s.render(w, 200, "help", s.page(r, u, "Help", nil))
}

type signinData struct {
	Error string
	Next  string
	Email string
}

// safeNext returns n when it is a plain local path, and /drafts otherwise. A path with a control character,
// a backslash, a scheme or a host is refused, because a browser can read some of them as another site.
func safeNext(n string) string {
	if !strings.HasPrefix(n, "/") || strings.HasPrefix(n, "//") || strings.Contains(n, "\\") {
		return "/drafts"
	}
	for _, r := range n {
		if r < 0x20 || r == 0x7f {
			return "/drafts"
		}
	}
	if u, err := url.Parse(n); err != nil || u.Scheme != "" || u.Host != "" {
		return "/drafts"
	}
	return n
}

func (s *Server) signinForm(w http.ResponseWriter, r *http.Request, u *User) {
	s.render(w, 200, "signin", s.page(r, u, "Sign in", signinData{Next: safeNext(r.URL.Query().Get("next"))}))
}

func (s *Server) signinPost(w http.ResponseWriter, r *http.Request, _ *User) {
	_ = r.ParseForm()
	email := strings.ToLower(strings.TrimSpace(r.PostFormValue("email")))
	next := safeNext(r.PostFormValue("next"))
	now := s.now()
	recent := s.fails[email][:0]
	for _, t := range s.fails[email] {
		if now.Sub(t) < time.Minute {
			recent = append(recent, t)
		}
	}
	s.fails[email] = recent
	if len(recent) >= 5 {
		s.render(w, http.StatusTooManyRequests, "signin", s.page(r, nil, "Sign in", signinData{
			Error: "Too many tries. Wait one minute and try again.", Next: next, Email: email}))
		return
	}
	for _, u := range s.users {
		if u.Email == email && r.PostFormValue("password") == TestPassword {
			s.startSession(w, u)
			// next went through safeNext, so it is a local path.
			http.Redirect(w, r, next, http.StatusSeeOther) //nolint:gosec // next is a local path from safeNext // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
			return
		}
	}
	s.fails[email] = append(s.fails[email], now)
	if s.has("bug-signin-silent") {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.render(w, http.StatusUnauthorized, "signin", s.page(r, nil, "Sign in", signinData{
		Error: "The email or password is not right.", Next: next, Email: email}))
}

func (s *Server) signout(w http.ResponseWriter, r *http.Request, _ *User) {
	if c, err := r.Cookie("inkwell_session"); err == nil {
		delete(s.sessions, c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "inkwell_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode}) //nolint:gosec // Inkwell is served over plain HTTP on localhost, so Secure would drop the cookie // nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure
	redirectMsg(w, r, "/", "You are signed out.")
}

// DraftRow is one line of the draft list.
type DraftRow struct {
	D          *Draft
	Owner      string
	StatusText string
	CanEdit    bool
}

type draftListData struct {
	Rows       []DraftRow
	Q          string
	Page, Last int
	NextPage   int
	PrevPage   int
	CanCreate  bool
	CanDelete  bool
}

func statusText(status string) string {
	return strings.ReplaceAll(status, "_", " ")
}

func (s *Server) draftList(w http.ResponseWriter, r *http.Request, u *User) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	var all []*Draft
	for _, d := range s.drafts {
		if q != "" && !s.has("bug-search-ignored") && !strings.Contains(strings.ToLower(d.Title), strings.ToLower(q)) {
			continue
		}
		all = append(all, d)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	const size = 3
	last := (len(all) + size - 1) / size
	if last == 0 {
		last = 1
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	if page > last {
		page = last
	}
	lo, hi := (page-1)*size, page*size
	if hi > len(all) {
		hi = len(all)
	}
	data := draftListData{Q: q, Page: page, Last: last, PrevPage: page - 1, NextPage: page + 1,
		CanCreate: in(u.Role, Author, Editor, Admin), CanDelete: s.canDelete(u)}
	if s.has("bug-page-next-loop") {
		data.NextPage = 1
	}
	for _, d := range all[lo:hi] {
		owner := ""
		if o := s.userByID(d.OwnerID); o != nil {
			owner = o.Name
		}
		st := d.Status
		if s.has("bug-list-stale-status") && st == "published" {
			st = "approved"
		}
		data.Rows = append(data.Rows, DraftRow{D: d, Owner: owner, StatusText: statusText(st), CanEdit: s.canEdit(u, d)})
	}
	s.render(w, 200, "drafts", s.page(r, u, "Drafts", data))
}

type draftData struct {
	D           *Draft
	Owner       string
	Reviewer    string
	ReviewerSee bool
	CanEdit     bool
	CanSubmit   bool
	CanReview   bool
	CanPublish  bool
	CanDelete   bool
	Locked      bool
	Error       string
	Title, Body string
}

func (s *Server) draftData(u *User, d *Draft) draftData {
	dd := draftData{D: d, CanEdit: s.canEdit(u, d), CanSubmit: s.canSubmit(u, d), CanReview: s.canReview(u, d),
		CanPublish: s.canPublish(u, d), CanDelete: s.canDelete(u), Locked: d.Status == "published",
		Title: d.Title, Body: d.Body}
	if o := s.userByID(d.OwnerID); o != nil {
		dd.Owner = o.Name
	}
	// Reviews are anonymous to the author. Staff see the reviewer's name.
	if rv := s.userByID(d.ReviewerID); rv != nil && in(u.Role, Editor, Reviewer, Admin) {
		dd.Reviewer = rv.Name
		dd.ReviewerSee = true
	}
	return dd
}

func (s *Server) draftShow(w http.ResponseWriter, r *http.Request, u *User) {
	d := s.draftByID(r.PathValue("id"))
	if d == nil {
		s.render(w, 404, "notfound", s.page(r, u, "Not found", nil))
		return
	}
	s.render(w, 200, "draft", s.page(r, u, d.Title, s.draftData(u, d)))
}

func (s *Server) draftNewForm(w http.ResponseWriter, r *http.Request, u *User) {
	if !in(u.Role, Author, Editor, Admin) {
		s.forbidden(w, r, u, "Your role cannot create drafts.")
		return
	}
	s.render(w, 200, "edit", s.page(r, u, "New draft", draftData{D: &Draft{}}))
}

func (s *Server) validTitle(t string) bool { return len(strings.TrimSpace(t)) >= 3 }

func (s *Server) titleError() string {
	if s.has("fric-error-vague") {
		return "Invalid."
	}
	return "Enter a title of at least 3 characters."
}

func (s *Server) draftNewPost(w http.ResponseWriter, r *http.Request, u *User) {
	if !in(u.Role, Author, Editor, Admin) {
		s.forbidden(w, r, u, "Your role cannot create drafts.")
		return
	}
	_ = r.ParseForm()
	title, body := strings.TrimSpace(r.PostFormValue("title")), r.PostFormValue("body")
	if !s.validTitle(title) {
		s.render(w, 422, "edit", s.page(r, u, "New draft", draftData{D: &Draft{}, Title: title, Body: body, Error: s.titleError()}))
		return
	}
	s.nextID++
	d := &Draft{ID: s.nextID, Title: title, Body: body, OwnerID: u.ID, Status: "draft", Words: len(strings.Fields(body))}
	s.drafts = append(s.drafts, d)
	redirectMsg(w, r, fmt.Sprintf("/drafts/%d", d.ID), "Draft created.")
}

func (s *Server) draftEditForm(w http.ResponseWriter, r *http.Request, u *User) {
	d := s.draftByID(r.PathValue("id"))
	if d == nil {
		s.render(w, 404, "notfound", s.page(r, u, "Not found", nil))
		return
	}
	if !s.canEdit(u, d) {
		msg := "You cannot edit this draft."
		if d.Status == "published" {
			msg = "Published drafts are locked."
		}
		s.forbidden(w, r, u, msg)
		return
	}
	dd := s.draftData(u, d)
	s.render(w, 200, "edit", s.page(r, u, "Edit "+d.Title, dd))
}

func (s *Server) draftEditPost(w http.ResponseWriter, r *http.Request, u *User) {
	d := s.draftByID(r.PathValue("id"))
	if d == nil {
		s.render(w, 404, "notfound", s.page(r, u, "Not found", nil))
		return
	}
	if !s.canEdit(u, d) {
		s.forbidden(w, r, u, "You cannot edit this draft.")
		return
	}
	_ = r.ParseForm()
	title, body := strings.TrimSpace(r.PostFormValue("title")), r.PostFormValue("body")
	if !s.validTitle(title) {
		dd := s.draftData(u, d)
		dd.Title, dd.Body, dd.Error = title, body, s.titleError()
		s.render(w, 422, "edit", s.page(r, u, "Edit "+d.Title, dd))
		return
	}
	if !s.has("bug-edit-title-lost") {
		d.Title = title
	}
	d.Body = body
	d.Words = len(strings.Fields(body))
	redirectMsg(w, r, fmt.Sprintf("/drafts/%d", d.ID), "Draft saved.")
}

func (s *Server) draftSubmit(w http.ResponseWriter, r *http.Request, u *User) {
	d := s.draftByID(r.PathValue("id"))
	if d == nil {
		s.render(w, 404, "notfound", s.page(r, u, "Not found", nil))
		return
	}
	if !s.canSubmit(u, d) {
		s.forbidden(w, r, u, "Only the owner can submit a draft, and only while it is a draft.")
		return
	}
	if !s.has("bug-submit-noop") {
		d.Status = "in_review"
	}
	redirectMsg(w, r, fmt.Sprintf("/drafts/%d", d.ID), "Submitted for review.")
}

func (s *Server) draftReview(w http.ResponseWriter, r *http.Request, u *User) {
	d := s.draftByID(r.PathValue("id"))
	if d == nil {
		s.render(w, 404, "notfound", s.page(r, u, "Not found", nil))
		return
	}
	if !s.canReview(u, d) {
		s.forbidden(w, r, u, "You cannot review this draft. Nobody reviews their own work.")
		return
	}
	_ = r.ParseForm()
	verdict := r.PostFormValue("verdict")
	if verdict != "approve" && verdict != "reject" {
		http.Error(w, "verdict must be approve or reject", http.StatusBadRequest)
		return
	}
	d.ReviewerID = u.ID
	d.Status = map[string]string{"approve": "approved", "reject": "rejected"}[verdict]
	s.reviews = append(s.reviews, &Review{DraftID: d.ID, ReviewerID: u.ID, Verdict: verdict})
	redirectMsg(w, r, fmt.Sprintf("/drafts/%d", d.ID), "Review saved.")
}

func (s *Server) draftPublish(w http.ResponseWriter, r *http.Request, u *User) {
	d := s.draftByID(r.PathValue("id"))
	if d == nil {
		s.render(w, 404, "notfound", s.page(r, u, "Not found", nil))
		return
	}
	if !s.canPublish(u, d) {
		s.forbidden(w, r, u, "You cannot publish this draft. It must be approved, and a different person must publish it.")
		return
	}
	d.Status = "published"
	redirectMsg(w, r, fmt.Sprintf("/drafts/%d", d.ID), "Published.")
}

func (s *Server) draftDelete(w http.ResponseWriter, r *http.Request, u *User) {
	d := s.draftByID(r.PathValue("id"))
	if d == nil {
		s.render(w, 404, "notfound", s.page(r, u, "Not found", nil))
		return
	}
	if !s.canDelete(u) {
		s.forbidden(w, r, u, "Only an admin can delete a draft.")
		return
	}
	for i, x := range s.drafts {
		if x == d {
			s.drafts = append(s.drafts[:i], s.drafts[i+1:]...)
			break
		}
	}
	redirectMsg(w, r, "/drafts", "Draft deleted.")
}

type reviewRow struct {
	Draft, Reviewer, Verdict string
	DraftID                  int
}

func (s *Server) reviewList(w http.ResponseWriter, r *http.Request, u *User) {
	if !s.canSeeReviews(u) {
		s.forbidden(w, r, u, "Your role cannot see the reviews page.")
		return
	}
	var rows []reviewRow
	for _, rv := range s.reviews {
		row := reviewRow{DraftID: rv.DraftID, Verdict: rv.Verdict}
		if d := s.draftByID(strconv.Itoa(rv.DraftID)); d != nil {
			row.Draft = d.Title
		}
		if x := s.userByID(rv.ReviewerID); x != nil {
			row.Reviewer = x.Name
		}
		rows = append(rows, row)
	}
	title := "Reviews"
	if s.has("a11y-reviews-notitle") {
		title = ""
	}
	s.render(w, 200, "reviews", s.page(r, u, title, rows))
}

type issueData struct {
	Issues    []*Issue
	CanCreate bool
	Error     string
	Title     string
	Date      string
}

func (s *Server) issueList(w http.ResponseWriter, r *http.Request, u *User) {
	s.render(w, 200, "issues", s.page(r, u, "Issues", issueData{Issues: s.issues, CanCreate: in(u.Role, Editor, Admin)}))
}

func (s *Server) issueCreate(w http.ResponseWriter, r *http.Request, u *User) {
	if !in(u.Role, Editor, Admin) {
		s.forbidden(w, r, u, "Only an editor or an admin can schedule an issue.")
		return
	}
	_ = r.ParseForm()
	title, raw := strings.TrimSpace(r.PostFormValue("title")), strings.TrimSpace(r.PostFormValue("date"))
	layout := "2006-01-02"
	if s.has("fric-date-format") {
		layout = "02-01-2006"
	}
	fail := func(msg string) {
		s.render(w, 422, "issues", s.page(r, u, "Issues", issueData{Issues: s.issues, CanCreate: true, Error: msg, Title: title, Date: raw}))
	}
	if !s.validTitle(title) {
		fail(s.titleError())
		return
	}
	t, err := time.Parse(layout, raw)
	if err != nil {
		if s.has("fric-date-format") {
			fail("Invalid date.")
		} else {
			fail("Enter the date as YYYY-MM-DD, for example 2027-03-01.")
		}
		return
	}
	if t.Before(s.now().UTC().Truncate(24 * time.Hour)) {
		if s.has("bug-issue-past-500") {
			http.Error(w, "internal error: schedule: nil slot", http.StatusInternalServerError)
			return
		}
		fail("The date must be today or later.")
		return
	}
	s.nextID++
	s.issues = append(s.issues, &Issue{ID: s.nextID, Title: title, Date: t, Slots: 2})
	redirectMsg(w, r, "/issues", "Issue scheduled.")
}

type payRow struct {
	P      *Payment
	Draft  string
	Author string
	Amount string
}

type payData struct {
	Rows     []payRow
	Total    string
	CanAdmin bool
}

func pounds(p int) string { return fmt.Sprintf("£%d.%02d", p/100, p%100) }

func (s *Server) paymentList(w http.ResponseWriter, r *http.Request, u *User) {
	if !s.canSeePayments(u) {
		s.forbidden(w, r, u, "Your role cannot see payments.")
		return
	}
	var rows []payRow
	sum := 0
	for _, p := range s.payments {
		if u.Role == Author && p.AuthorID != u.ID {
			continue
		}
		row := payRow{P: p, Amount: pounds(p.Pence)}
		if d := s.draftByID(strconv.Itoa(p.DraftID)); d != nil {
			row.Draft = d.Title
		}
		if a := s.userByID(p.AuthorID); a != nil {
			row.Author = a.Name
		}
		rows = append(rows, row)
		sum += p.Pence
	}
	if s.has("bug-payments-total") && len(rows) > 1 {
		sum -= rows[len(rows)-1].P.Pence
	}
	s.render(w, 200, "payments", s.page(r, u, "Payments", payData{Rows: rows, Total: pounds(sum), CanAdmin: u.Role == Admin}))
}

func (s *Server) paymentApprove(w http.ResponseWriter, r *http.Request, u *User) {
	if u.Role != Admin {
		s.forbidden(w, r, u, "Only an admin can approve a payment.")
		return
	}
	id, _ := strconv.Atoi(r.PathValue("id"))
	for _, p := range s.payments {
		if p.ID == id {
			p.Status = "approved"
			redirectMsg(w, r, "/payments", "Payment approved.")
			return
		}
	}
	s.render(w, 404, "notfound", s.page(r, u, "Not found", nil))
}

func (s *Server) userList(w http.ResponseWriter, r *http.Request, u *User) {
	if u.Role != Admin && (u.Role != Editor || !s.has("perm-editor-users")) {
		s.forbidden(w, r, u, "Only an admin can manage users.")
		return
	}
	s.render(w, 200, "users", s.page(r, u, "Users", s.users))
}

func (s *Server) userRole(w http.ResponseWriter, r *http.Request, u *User) {
	if u.Role != Admin {
		s.forbidden(w, r, u, "Only an admin can change a role.")
		return
	}
	_ = r.ParseForm()
	role := r.PostFormValue("role")
	if !in(role, Viewer, Author, Editor, Reviewer, Admin) {
		http.Error(w, "unknown role", http.StatusBadRequest)
		return
	}
	target := s.userByID(r.PathValue("id"))
	if target == nil {
		s.render(w, 404, "notfound", s.page(r, u, "Not found", nil))
		return
	}
	target.Role = role
	redirectMsg(w, r, "/admin/users", "Role changed.")
}

func (s *Server) settingsForm(w http.ResponseWriter, r *http.Request, u *User) {
	if u.Role != Admin && (u.Role != Editor || !s.has("perm-editor-settings")) {
		s.forbidden(w, r, u, "Only an admin can change settings.")
		return
	}
	s.render(w, 200, "settings", s.page(r, u, "Settings", nil))
}

func (s *Server) settingsPost(w http.ResponseWriter, r *http.Request, u *User) {
	if u.Role != Admin && (u.Role != Editor || !s.has("perm-editor-settings")) {
		s.forbidden(w, r, u, "Only an admin can change settings.")
		return
	}
	_ = r.ParseForm()
	if name := strings.TrimSpace(r.PostFormValue("site_name")); name != "" && len(name) <= 40 {
		s.siteName = name
	}
	redirectMsg(w, r, "/settings", "Settings saved.")
}

func (s *Server) testLoginHandler(w http.ResponseWriter, r *http.Request, _ *User) {
	role := r.URL.Query().Get("as")
	who := r.URL.Query().Get("user")
	for _, u := range s.users {
		if (who != "" && u.ID == who) || (who == "" && u.Role == role) {
			s.startSession(w, u)
			// safeNext only lets a local path through.
			http.Redirect(w, r, safeNext(r.URL.Query().Get("next")), http.StatusSeeOther) //nolint:gosec // local path from safeNext // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
			return
		}
	}
	http.Error(w, "no such test user", http.StatusNotFound)
}

func (s *Server) reset(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seed()
	fmt.Fprintln(w, "reset")
}

var _ = url.QueryEscape

const coverSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="48" height="48" viewBox="0 0 48 48"><rect width="48" height="48" fill="#173a5e"/><path d="M12 36l10-24 4 10 10-2-8 16z" fill="#fff"/></svg>`

func coverHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	_, _ = w.Write([]byte(coverSVG))
}
