package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dummy-run/dummy-run/casts"
	"github.com/dummy-run/dummy-run/internal/cast"
)

// ListPersonasInput takes no arguments.
type ListPersonasInput struct{}

// ScenarioInfo names one scenario of a persona.
type ScenarioInfo struct {
	ID    string `json:"id" jsonschema:"scenario ID to pass to run_persona"`
	Title string `json:"title"`
}

// PersonaInfo is one persona in the list.
type PersonaInfo struct {
	ID        string         `json:"id"`
	Name      string         `json:"name,omitempty"`
	Source    string         `json:"source" jsonschema:"library, or the name of the local cast folder"`
	Role      string         `json:"role"`
	Device    string         `json:"device" jsonschema:"desktop, tablet or phone"`
	Summary   string         `json:"summary"`
	Scenarios []ScenarioInfo `json:"scenarios"`
}

// ListPersonasOutput is the list_personas result.
type ListPersonasOutput struct {
	Personas []PersonaInfo `json:"personas"`
	Warnings []string      `json:"warnings,omitempty" jsonschema:"personas that could not be read"`
}

// root is a folder of personas and the name that goes in the Source field.
type root struct{ source, dir string }

func (s *Server) roots() ([]root, error) {
	var out []root
	for _, d := range s.cfg.CastDirs {
		out = append(out, root{source: filepath.Base(d), dir: d})
	}
	lib, err := s.libraryDir()
	if err != nil {
		return nil, err
	}
	return append(out, root{source: "library", dir: lib}), nil
}

func (s *Server) libraryDir() (string, error) {
	if s.cfg.LibraryDir != "" {
		return s.cfg.LibraryDir, nil
	}
	return ExtractLibrary()
}

// ExtractLibrary writes the persona library built into the program to the user's cache folder and
// returns it. The folder name holds a hash of the content, so a new version of the program gets its own
// copy and an existing copy is reused.
func ExtractLibrary() (string, error) {
	h := sha256.New()
	err := fs.WalkDir(casts.Library, "library", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := casts.Library.ReadFile(p)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", p, len(data))
		h.Write(data)
		return nil
	})
	if err != nil {
		return "", err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = os.TempDir()
	}
	parent := filepath.Join(cache, "dummyrun")
	final := filepath.Join(parent, "library-"+hex.EncodeToString(h.Sum(nil))[:12])
	marker := filepath.Join(final, "first-time-visitor", "profile.md")
	if _, err := os.Stat(marker); err == nil {
		return final, nil
	}
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(parent, "library-new-")
	if err != nil {
		return "", err
	}
	err = fs.WalkDir(casts.Library, "library", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dst := filepath.Join(tmp, filepath.FromSlash(strings.TrimPrefix(strings.TrimPrefix(p, "library"), "/")))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o750)
		}
		data, err := casts.Library.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o600)
	})
	if err != nil {
		return "", err
	}
	if err := os.Rename(tmp, final); err != nil {
		// Another process won the race. Its copy has the same content.
		if _, serr := os.Stat(marker); serr == nil {
			return final, nil
		}
		return "", err
	}
	return final, nil
}

func (s *Server) listPersonas(ctx context.Context, _ *mcp.CallToolRequest, _ ListPersonasInput) (*mcp.CallToolResult, ListPersonasOutput, error) {
	_, done := s.begin(ctx)
	defer done()
	roots, err := s.roots()
	if err != nil {
		return nil, ListPersonasOutput{}, fmt.Errorf("cannot read the persona library: %s", s.redact(err.Error()))
	}
	out := ListPersonasOutput{Personas: []PersonaInfo{}}
	seen := map[string]bool{}
	for _, r := range roots {
		ids, err := cast.List(r.dir)
		if err != nil {
			if !os.IsNotExist(err) {
				out.Warnings = append(out.Warnings, fmt.Sprintf("%s: %s", r.source, s.redact(err.Error())))
			}
			continue
		}
		sort.Strings(ids)
		for _, id := range ids {
			if seen[id] {
				continue // an earlier cast replaces a later one with the same ID
			}
			info, err := describe(r, id)
			if err != nil {
				out.Warnings = append(out.Warnings, fmt.Sprintf("%s/%s: %s", r.source, id, err))
				continue
			}
			seen[id] = true
			out.Personas = append(out.Personas, info)
		}
	}
	return nil, out, nil
}

func describe(r root, id string) (PersonaInfo, error) {
	p, err := cast.Load(r.dir, id)
	if err != nil {
		return PersonaInfo{}, err
	}
	info := PersonaInfo{ID: p.ID, Name: p.Name, Source: r.source, Role: p.Role, Device: p.Device, Scenarios: []ScenarioInfo{}}
	data, err := os.ReadFile(filepath.Join(r.dir, id, "profile.md")) //nolint:gosec // the cast folder is configured on purpose
	if err != nil {
		return PersonaInfo{}, err
	}
	meta, body, _ := cast.FrontMatter(string(data))
	info.Summary = meta["summary"]
	if info.Summary == "" {
		info.Summary = firstSentence(body)
	}
	for _, sc := range p.Scenarios {
		info.Scenarios = append(info.Scenarios, ScenarioInfo{ID: sc.ID, Title: sc.Title})
	}
	return info, nil
}

// firstSentence is the fallback summary: the first paragraph that is not a heading or a list.
func firstSentence(body string) string {
	for _, para := range strings.Split(body, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" || strings.HasPrefix(para, "#") || strings.HasPrefix(para, "-") {
			continue
		}
		para = strings.Join(strings.Fields(para), " ")
		if i := strings.Index(para, ". "); i > 0 {
			para = para[:i+1]
		}
		return truncate(para, 160)
	}
	return ""
}

// findPersona loads a persona from the first folder that has it.
func (s *Server) findPersona(id string) (*cast.Persona, error) {
	roots, err := s.roots()
	if err != nil {
		return nil, err
	}
	var known []string
	for _, r := range roots {
		ids, err := cast.List(r.dir)
		if err != nil {
			continue
		}
		for _, have := range ids {
			if have == id {
				return cast.Load(r.dir, id)
			}
			known = append(known, have)
		}
	}
	sort.Strings(known)
	return nil, fmt.Errorf("no persona %q. The personas are %s. Call list_personas for details", id, strings.Join(known, ", "))
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-3]) + "..."
}
