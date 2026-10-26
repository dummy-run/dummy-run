package a11y

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/visual"
)

// maskJS covers each element the selectors match with a solid box fixed over it, so parts of the page that change
// on every load, such as a clock or an advert, don't count as changes. It reads every selector before it draws a
// box, so a selector that isn't valid CSS leaves the page as it was. It returns how many it covered.
const maskJS = `(selectors) => {
  const rects = [];
  for (const sel of JSON.parse(selectors)) {
    let els;
    try { els = document.querySelectorAll(sel); } catch (e) { throw new Error("--mask " + JSON.stringify(sel) + " is not a CSS selector"); }
    for (const el of els) {
      const r = el.getBoundingClientRect();
      if (r.width > 0 && r.height > 0) rects.push(r);
    }
  }
  for (const r of rects) {
    const box = document.createElement("div");
    box.setAttribute("data-dummyrun-mask", "");
    box.style.cssText = "all:initial;position:fixed;z-index:2147483647;pointer-events:none;background:#ff00ff;" +
      "left:" + r.left + "px;top:" + r.top + "px;width:" + r.width + "px;height:" + r.height + "px";
    document.documentElement.appendChild(box);
  }
  return rects.length;
}`

const unmaskJS = `() => { for (const el of document.querySelectorAll("[data-dummyrun-mask]")) el.remove(); }`

// compareScreen takes a picture of the window with the masks drawn, compares it with the one the last sweep left in
// dir, and leaves the new one in its place. It reports the counts and judges nothing.
func compareScreen(ctx context.Context, conn *bidi.Conn, cx, dir string, masks []string, res *PageResult) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	sels, err := json.Marshal(append([]string{}, masks...))
	if err != nil {
		return err
	}
	var masked int
	// The boxes come off whatever happens, since the checks after this one measure the same page.
	defer func() { _, _ = conn.CallFunction(context.WithoutCancel(ctx), cx, unmaskJS) }()
	if err := conn.CallFunctionJSON(ctx, cx, maskJS, &masked, string(sels)); err != nil {
		var se *bidi.ScriptError
		if errors.As(err, &se) {
			if i := strings.Index(se.Text, "--mask"); i >= 0 {
				return errors.New(se.Text[i:])
			}
		}
		return err
	}
	data, err := conn.CaptureScreenshot(ctx, cx)
	if err != nil {
		return err
	}
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return err
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("read the screenshot: %w", err)
	}
	path := screenName(dir, res)
	v := &Visual{Screen: path, Masked: masked}
	if _, err := os.Stat(path); err == nil {
		// An earlier picture that can't be read is replaced, so one bad file doesn't stop every later sweep.
		switch c, err := visual.Compare(path, img, strings.TrimSuffix(path, ".png")+".diff.png"); {
		case errors.Is(err, visual.ErrUnreadable):
			v.First = true
		case err != nil:
			return err
		default:
			v.Change = c
		}
	} else if !os.IsNotExist(err) {
		return err
	} else {
		v.First = true
	}
	if err := visual.WriteFile(path, img); err != nil {
		return err
	}
	res.Visual = v
	return nil
}

// Visual is how a page looked against the last sweep that wrote to the same --visual directory.
type Visual struct {
	// Screen is the picture of the window this sweep took.
	Screen string `json:"screen"`
	// Masked counts the elements --mask covered.
	Masked int `json:"masked,omitempty"`
	// First says there was no earlier picture to compare with.
	First bool `json:"first,omitempty"`
	*visual.Change
}

// screenName is the picture for one page and setting. Like screenshotName, it ends the address with a short hash.
func screenName(dir string, res *PageResult) string {
	base := unsafeName.ReplaceAllString(strings.TrimPrefix(strings.TrimPrefix(res.URL, "https://"), "http://"), "-")
	if len(base) > 60 {
		base = base[:60]
	}
	sum := sha256.Sum256([]byte(res.URL))
	setting := strings.ReplaceAll(res.Setting(), "/", "-")
	return filepath.Join(dir, fmt.Sprintf("%s-%s-%s.png", base, hex.EncodeToString(sum[:4]), setting))
}
