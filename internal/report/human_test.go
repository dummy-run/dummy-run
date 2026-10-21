package report

import "testing"

func TestWithArticleBySound(t *testing.T) {
	for word, want := range map[string]string{
		"user": "a user", "User": "a User", "union": "a union", "one-off": "a one-off", "editor": "an editor",
		"admin": "an admin", "contributor": "a contributor", "": "a ",
	} {
		if got := withArticle(word); got != want {
			t.Errorf("withArticle(%q) = %q, want %q", word, got, want)
		}
	}
}

func TestReproStepsOmitsMissingSetup(t *testing.T) {
	steps := reproSteps(&Finding{})
	if len(steps) != 1 || steps[0] != "Use a browser." {
		t.Fatalf("empty reproduction gave %q", steps)
	}
}

func TestPlaceFallbacks(t *testing.T) {
	for route, want := range map[string]string{"/": "the Home page", "/:id": "the Home page", "/draft-list": "the Draft list page"} {
		if got := pageName(route); got != want {
			t.Errorf("pageName(%q) = %q, want %q", route, got, want)
		}
	}
	if got := locusText("page"); got != "" {
		t.Errorf("locusText(page) = %q, want empty", got)
	}
}
