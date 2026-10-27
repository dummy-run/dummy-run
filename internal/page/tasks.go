package page

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

//go:embed tasks.js
var tasksJS string

const tasksSandbox = "dummyrun-tasks"

// MaxTasks is the most controls one page tries by mouse and by keyboard. Each try loads the page twice.
const MaxTasks = 5

// taskSettle is how long the page gets to react after an action before its success test is read again.
const taskSettle = 400 * time.Millisecond

// Task is one control tried by mouse and by keyboard. Kind says how its success is tested: "expanded" (its
// aria-expanded becomes true), "controls" (the element its aria-controls names is shown), "details" (its <details>
// opens), "selected" (a tab becomes selected), "checked" (a custom checkbox or switch becomes checked) or "change"
// (the address, the words or the shown elements of the page change, for something that looks clickable but is not a
// control).
type Task struct {
	Selector string `json:"selector"`
	Kind     string `json:"kind"`
	Target   string `json:"target,omitempty"`
	Label    string `json:"label,omitempty"`
	// Mouse says a click on the element made the success test true. Keyboard says Enter or Space did, after Tab
	// reached the element. Reached says Tab reached it at all.
	Mouse    bool `json:"mouse"`
	Keyboard bool `json:"keyboard"`
	Reached  bool `json:"reached"`
	// Skipped says why the task has no verdict: the test was already true on a fresh page, the element was
	// covered so a mouse could not press it, the click did nothing either, or Tab ran out of stops first.
	Skipped string `json:"skipped,omitempty"`
}

// MouseOnly reports whether the task works by mouse and not by keyboard.
func (t Task) MouseOnly() bool { return t.Skipped == "" && t.Mouse && !t.Keyboard }

// SkipTry is a skip link tried by keyboard: Enter on it, then Tab. Moved says focus then landed inside its target
// or after it.
type SkipTry struct {
	Link   string `json:"link"`
	Target string `json:"target"`
	Label  string `json:"label,omitempty"`
	Moved  bool   `json:"moved"`
}

// Tasks is what the keyboard and mouse comparison found on one page.
type Tasks struct {
	Tasks []Task   `json:"tasks"`
	Skip  *SkipTry `json:"skip,omitempty"`
	// Responses are how long the page took to respond to each click, Enter and Space, as web-vitals measures INP.
	Responses []Response `json:"responses,omitempty"`
}

type taskState struct {
	Found    bool `json:"found"`
	Pass     bool `json:"pass"`
	X        int  `json:"x"`
	Y        int  `json:"y"`
	Hittable bool `json:"hittable"`
}

func taskCall(ctx context.Context, conn *bidi.Conn, cx, op string, arg, out any) error {
	return conn.CallFunctionJSONIn(ctx, cx, tasksSandbox, tasksJS, out, op, arg)
}

// focusTries is how many Tab presses load gives focus to come back into the page. Firefox's own controls take more
// than ten stops in a desktop-width window, and the walk can leave focus anywhere among them.
const focusTries = 40

// load opens the page afresh, so each try starts from the same state, and presses Tab until focus is in the page,
// up to focusTries times.
func load(ctx context.Context, conn *bidi.Conn, cx, url string) error {
	if _, err := conn.Navigate(ctx, cx, url); err != nil {
		return err
	}
	// Keys need the page to have focus, or Enter on a button does nothing in Firefox. Focus can be left in the
	// browser's own controls, such as the address bar after Tab has run off the end of a page, and then Firefox
	// keeps it there across loads. Tab moves it back into the page, and loading the page again starts the next Tab
	// from the top.
	// A page that never takes focus is an error, not a control that Enter and Space did nothing to.
	tab, _ := bidi.Key("Tab")
	for i := 0; ; i++ {
		var has bool
		if err := conn.CallFunctionJSON(ctx, cx, "() => document.hasFocus()", &has); err != nil {
			return err
		}
		if has {
			if i > 0 {
				if _, err := conn.Navigate(ctx, cx, url); err != nil {
					return err
				}
			}
			break
		}
		if i == focusTries {
			return errors.New("the page never took keyboard focus")
		}
		if err := conn.PressKey(ctx, cx, tab); err != nil {
			return err
		}
	}
	return sleep(ctx, taskSettle)
}

func sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TryTasks tries up to limit controls on the page at url, each by mouse and then by keyboard on a fresh load, and the
// skip link by keyboard. It loads the page many times. Give it a tab of its own that has not had keys pressed in it:
// once focus has left a page for the browser's own controls, as it does at the end of a Tab walk, Firefox no longer
// runs Enter's default action in that tab. Nothing that would leave the page or send a form is tried, but a click
// handler can still send a request, so don't call it when the browser is signed in. ready, if not nil, runs after
// each load and before anything is pressed.
func TryTasks(ctx context.Context, conn *bidi.Conn, cx, url string, limit int, ready func(context.Context) error) (*Tasks, error) {
	fresh := func() error {
		if err := load(ctx, conn, cx, url); err != nil {
			return err
		}
		if ready != nil {
			return ready(ctx)
		}
		return nil
	}
	if err := fresh(); err != nil {
		return nil, fmt.Errorf("tasks: %w", err)
	}
	found := []Task{}
	if limit > 0 {
		if err := taskCall(ctx, conn, cx, "find", strconv.Itoa(limit), &found); err != nil {
			return nil, fmt.Errorf("tasks: %w", err)
		}
	}
	tab, _ := bidi.Key("Tab")
	enter, _ := bidi.Key("Enter")
	space, _ := bidi.Key("Space")
	out := &Tasks{Tasks: []Task{}}
	for _, t := range found {
		if err := tryTask(ctx, conn, cx, fresh, &t, tab, enter, space, &out.Responses); err != nil {
			return nil, err
		}
		out.Tasks = append(out.Tasks, t)
	}
	sk, err := trySkip(ctx, conn, cx, fresh, tab, enter)
	if err != nil {
		return nil, err
	}
	out.Skip = sk
	return out, nil
}

func tryTask(ctx context.Context, conn *bidi.Conn, cx string, fresh func() error, t *Task, tab, enter, space string, responses *[]Response) error {
	spec, err := json.Marshal(struct {
		Selector string `json:"selector"`
		Kind     string `json:"kind"`
		Target   string `json:"target"`
	}{t.Selector, t.Kind, t.Target})
	if err != nil {
		return fmt.Errorf("tasks: %w", err)
	}
	arm := func() (taskState, error) {
		var st taskState
		if err := fresh(); err != nil {
			return st, fmt.Errorf("tasks: %w", err)
		}
		err := taskCall(ctx, conn, cx, "arm", string(spec), &st)
		startINP(ctx, conn, cx)
		return st, err
	}
	// check reads the success test once the page has had time to react, and how long the page took to respond.
	check := func(action string) (bool, error) {
		if err := sleep(ctx, taskSettle); err != nil {
			return false, err
		}
		var st taskState
		err := taskCall(ctx, conn, cx, "check", string(spec), &st)
		if r := readINP(ctx, conn, cx, action, t.Selector); r != nil {
			*responses = append(*responses, *r)
		}
		return st.Found && st.Pass, err
	}
	// By mouse.
	st, err := arm()
	if err != nil {
		return err
	}
	switch {
	case !st.Found:
		t.Skipped = "not found on a fresh load"
		return nil
	case st.Pass:
		t.Skipped = "the success test is already true on a fresh load"
		return nil
	case !st.Hittable:
		t.Skipped = "something else is on top of it, so a mouse cannot press it"
		return nil
	}
	markINP(ctx, conn, cx)
	if err := conn.Click(ctx, cx, float64(st.X), float64(st.Y)); err != nil {
		return fmt.Errorf("tasks: click: %w", err)
	}
	if t.Mouse, err = check("click"); err != nil {
		return err
	}
	if !t.Mouse {
		t.Skipped = "a click did not change it either"
		return nil
	}
	// By keyboard, from a fresh load: Tab until the element has focus, then Enter, then Space.
	if st, err = arm(); err != nil {
		return err
	}
	if !st.Found || st.Pass {
		t.Skipped = "the page differed on the second load"
		return nil
	}
	// The walk ends when focus comes back to the first stop it found, or stays out of the page for three presses,
	// so Tab has been everywhere. Chrome can start the walk part way down the page and pass through its own
	// controls before it comes round to the top, and Firefox can leave focus in its own controls. A walk that runs
	// out of stops first has no verdict.
	first, out, ended := "", 0, false
	for i := 0; i < MaxTabStops && !t.Reached && !ended; i++ {
		if err := conn.PressKey(ctx, cx, tab); err != nil {
			return fmt.Errorf("tasks: %w", err)
		}
		var f struct {
			On bool   `json:"on"`
			At string `json:"at"`
		}
		if err := taskCall(ctx, conn, cx, "focused", t.Selector, &f); err != nil {
			return fmt.Errorf("tasks: %w", err)
		}
		t.Reached = f.On
		switch {
		case f.At == "":
			out++
			ended = out >= 3
		case first == "":
			first, out = f.At, 0
		default:
			out = 0
			ended = f.At == first
		}
	}
	if !t.Reached {
		if !ended {
			t.Skipped = fmt.Sprintf("Tab did not reach it within %d stops", MaxTabStops)
		}
		return nil
	}
	for i, key := range []string{enter, space} {
		markINP(ctx, conn, cx)
		if err := conn.PressKey(ctx, cx, key); err != nil {
			return fmt.Errorf("tasks: %w", err)
		}
		if t.Keyboard, err = check([]string{"enter", "space"}[i]); err != nil || t.Keyboard {
			return err
		}
	}
	return nil
}

// trySkip presses Tab once on a fresh load. If focus is on a skip link with a shown target, it presses Enter and Tab
// and says whether focus then landed inside the target or after it. It gives no verdict when Tab alone gets there.
func trySkip(ctx context.Context, conn *bidi.Conn, cx string, fresh func() error, tab, enter string) (*SkipTry, error) {
	// First Tab alone from the link. If that already lands in or after the target, as it does when the link sits
	// just before it, the test can't tell a working link from one that does nothing.
	sk, err := armSkip(ctx, conn, cx, fresh, tab)
	if err != nil || sk == nil {
		return nil, err
	}
	var plain bool
	if err := pressAndCheck(ctx, conn, cx, sk.Target, &plain, tab); err != nil || plain {
		return nil, err
	}
	if sk, err = armSkip(ctx, conn, cx, fresh, tab); err != nil || sk == nil {
		return nil, err
	}
	if err := pressAndCheck(ctx, conn, cx, sk.Target, &sk.Moved, enter, tab); err != nil {
		return nil, err
	}
	return sk, nil
}

// armSkip loads the page and presses Tab once. It returns the skip link if that put focus on one whose target
// comes after it.
func armSkip(ctx context.Context, conn *bidi.Conn, cx string, fresh func() error, tab string) (*SkipTry, error) {
	if err := fresh(); err != nil {
		return nil, fmt.Errorf("tasks: %w", err)
	}
	if err := conn.PressKey(ctx, cx, tab); err != nil {
		return nil, fmt.Errorf("tasks: %w", err)
	}
	var sk *SkipTry
	if err := taskCall(ctx, conn, cx, "skip", nil, &sk); err != nil || sk == nil {
		return nil, err
	}
	// The test is false now: focus is on the link, before its target.
	var before bool
	if err := taskCall(ctx, conn, cx, "past", sk.Target, &before); err != nil {
		return nil, err
	}
	if before {
		return nil, nil // the target comes before the link, so the test can't tell
	}
	return sk, nil
}

// pressAndCheck presses keys in turn, then says whether focus is in or after target.
func pressAndCheck(ctx context.Context, conn *bidi.Conn, cx, target string, past *bool, keys ...string) error {
	for _, key := range keys {
		if err := conn.PressKey(ctx, cx, key); err != nil {
			return fmt.Errorf("tasks: %w", err)
		}
		if err := sleep(ctx, taskSettle/2); err != nil {
			return err
		}
	}
	return taskCall(ctx, conn, cx, "past", target, past)
}
