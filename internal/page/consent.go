package page

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

// DuckDuckGo autoconsent answers cookie banners. It is a pinned upstream build; see docs/TOOLS.md and NOTICE.
var (
	//go:embed consent/autoconsent.playwright.js
	autoconsentJS string
	//go:embed consent/compact-rules.json
	autoconsentRules string
	//go:embed consent.js
	consentJS string
)

// AutoconsentVersion is the version of the bundled autoconsent.
const AutoconsentVersion = "16.61.0"

// ConsentWait is how long the sweep waits for autoconsent to finish once it finds a banner.
const ConsentWait = 10 * time.Second

// ConsentButton is one button on a banner, with the kind autoconsent's heuristics give it: reject, accept,
// settings, acknowledge or other.
type ConsentButton struct {
	Text string `json:"text"`
	Kind string `json:"kind"`
}

// ConsentPopup is a banner autoconsent's heuristics found before anything was clicked.
type ConsentPopup struct {
	Selector string          `json:"selector"`
	Text     string          `json:"text"`
	Buttons  []ConsentButton `json:"buttons"`
}

// Consent is what happened to the page's cookie banner.
type Consent struct {
	Popups []ConsentPopup `json:"popups,omitempty"`
	// CMP names the consent platform autoconsent recognised, or a HEURISTIC name when it worked from the text.
	CMP      string `json:"cmp,omitempty"`
	Found    bool   `json:"found,omitempty"`
	OptedOut bool   `json:"opted_out,omitempty"`
	Done     bool   `json:"done,omitempty"`
	// Settled says autoconsent had finished, or had finished looking and found nothing, when the sweep moved on.
	// When it is false, autoconsent was still waiting for the page to change and could act later.
	Settled bool `json:"settled,omitempty"`
	// Cleared says every banner it found is gone or hidden afterwards, as measured in the page.
	Cleared bool `json:"cleared"`
	// Failed says autoconsent couldn't start or read the page, so the banner went unanswered. Error says why. An
	// Error without Failed is a problem autoconsent reported while it worked, such as a step that didn't match.
	Failed bool   `json:"failed,omitempty"`
	Error  string `json:"error,omitempty"`
	// Steps are autoconsent's messages in order, such as "popupFound Onetrust" and "optOutResult Onetrust true".
	Steps []string `json:"steps,omitempty"`
}

// HasKind reports whether the popup has a button of the kind.
func (p ConsentPopup) HasKind(kind string) bool {
	for _, b := range p.Buttons {
		if b.Kind == kind {
			return true
		}
	}
	return false
}

// AnswerConsent runs autoconsent in the loaded page: it reads any banner, then opts out and waits up to wait for that
// to finish. It runs in the page's own world, because a consent platform's API lives there.
func AnswerConsent(ctx context.Context, conn *bidi.Conn, cx string, wait time.Duration) (*Consent, error) {
	var out Consent
	script := strings.Replace(consentJS, "/*AUTOCONSENT*/", "\n"+autoconsentJS+"\n", 1)
	if err := conn.CallFunctionJSON(ctx, cx, script, &out, autoconsentRules, int(wait.Milliseconds())); err != nil {
		return nil, fmt.Errorf("cookie banner: %w", err)
	}
	return &out, nil
}
