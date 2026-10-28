package plain

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseOrdersByLine(t *testing.T) {
	got, err := parse([]byte(`{"/tmp/p.html":[{"Check":"DummyRun.Latin","Line":3,"Match":"e.g."},{"Check":"DummyRun.WordsToAvoid","Line":1,"Match":"leverage"}]}`))
	if err != nil || len(got) != 2 || got[0].Line != 1 || got[1].Check != "DummyRun.Latin" {
		t.Fatalf("%v %+v", err, got)
	}
	if _, err := parse([]byte("not json")); err == nil {
		t.Error("bad output should be an error")
	}
}

func TestFindHonoursTheVariable(t *testing.T) {
	t.Setenv("DUMMYRUN_VALE", "/no/such/vale")
	if _, err := Find(); err == nil || !strings.Contains(err.Error(), "DUMMYRUN_VALE") {
		t.Errorf("a missing DUMMYRUN_VALE: %v", err)
	}
	t.Setenv("DUMMYRUN_VALE", "")
	t.Setenv("PATH", t.TempDir())
	if _, err := Find(); !errors.Is(err, ErrNoVale) {
		t.Errorf("no vale on the PATH: %v", err)
	}
}

// TestCheckFindsPlantedProblems runs the real Vale when one is installed.
func TestCheckFindsPlantedProblems(t *testing.T) {
	vale, err := Find()
	if err != nil {
		t.Skip("vale is not installed; set DUMMYRUN_VALE to run this test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	html := `<html lang="en"><body><h1>Help</h1><p>We will leverage our tools to help you, e.g. by phone. ` +
		`This sentence goes on and on with far more words than anyone needs so that it runs well past the twenty five word limit the guide suggests.</p>` +
		`<pre>utilise e.g. code</pre></body></html>`
	got, err := Check(ctx, vale, html)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, a := range got {
		seen[a.Check]++
	}
	for _, c := range []string{"DummyRun.WordsToAvoid", "DummyRun.Latin", "DummyRun.LongSentence"} {
		if seen[c] != 1 {
			t.Errorf("%s: %d alerts, want 1: %+v", c, seen[c], got)
		}
	}
	clean, err := Check(ctx, vale, `<html lang="en"><body><h1>Opening hours</h1><p>We are open from 9am to 5pm. Call us if you need help.</p></body></html>`)
	if err != nil || len(clean) != 0 {
		t.Errorf("plain text has alerts: %v %+v", err, clean)
	}
}

func TestValeVersionMatchesDockerfile(t *testing.T) {
	b, err := os.ReadFile("../../docker/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "\nARG VALE_VERSION="+ValeVersion+"\n") {
		t.Errorf("docker/Dockerfile does not pin Vale %s", ValeVersion)
	}
}

// TestCheckFindsHardText runs the real Vale on text that is hard to read, and checks both readability rules fire and
// say which way to go.
func TestCheckFindsHardText(t *testing.T) {
	vale, err := Find()
	if err != nil {
		t.Skip("vale is not installed; set DUMMYRUN_VALE to run this test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	html := `<html lang="en"><body><h1>Terms</h1><p>Notwithstanding contractual obligations, organisational ` +
		`representatives systematically communicate preliminary documentation. Administrative considerations ` +
		`necessitate comprehensive interdepartmental consultation. Implementation responsibilities accompany ` +
		`institutional accountability requirements.</p></body></html>`
	got, err := Check(ctx, vale, html)
	if err != nil {
		t.Fatal(err)
	}
	msgs := map[string]string{}
	for _, a := range got {
		msgs[a.Check] = a.Message
	}
	if m := msgs["Readability.FleschReadingEase"]; !strings.Contains(m, "at 70 or above") {
		t.Errorf("reading ease: %q in %+v", m, got)
	}
	if m := msgs["Readability.FleschKincaid"]; !strings.Contains(m, "at 8 or below") {
		t.Errorf("grade: %q in %+v", m, got)
	}
}

// Cleanup removes the rules, and the next check writes them again.
func TestCleanupRemovesTheRules(t *testing.T) {
	cfg, err := configPath()
	if err != nil {
		t.Fatal(err)
	}
	Cleanup()
	if _, err := os.Stat(cfg); !os.IsNotExist(err) {
		t.Errorf("the rules are still there: %v", err)
	}
	again, err := configPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(again); err != nil {
		t.Errorf("the rules weren't written again: %v", err)
	}
	Cleanup()
}
