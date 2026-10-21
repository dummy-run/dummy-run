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
