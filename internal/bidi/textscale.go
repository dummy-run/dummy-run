package bidi

import "fmt"

// Text scale limits in percent. 100 is the browser's own size.
const (
	MinTextScale = 100
	MaxTextScale = 300
)

// The browsers' default font sizes in CSS pixels. A text scale multiplies them.
const (
	defaultVariableFontPx = 16
	defaultFixedFontPx    = 13
)

// TextScalePrefs returns the browser settings that make a user's "larger text" setting: the default
// font size, which is what rem and em units and unstyled text follow. It does not zoom the page
// and does not touch the minimum font size, so text that a page sets in px keeps its size, as it
// does for a person who raised only the default size. A scale of 100 or less returns nothing.
//
// Exactly one of the two maps is filled, for the browser given.
func TextScalePrefs(browser string, percent int) (chrome, firefox map[string]any, err error) {
	if percent < MinTextScale || percent > MaxTextScale {
		return nil, nil, fmt.Errorf("text scale %d percent is outside %d to %d", percent, MinTextScale, MaxTextScale)
	}
	variable := scaled(defaultVariableFontPx, percent)
	fixed := scaled(defaultFixedFontPx, percent)
	switch browser {
	case "chrome":
		if percent == MinTextScale {
			return nil, nil, nil
		}
		return map[string]any{"webkit": map[string]any{"webprefs": map[string]any{
			"default_font_size": variable, "default_fixed_font_size": fixed,
		}}}, nil, nil
	case "firefox":
		if percent == MinTextScale {
			return nil, nil, nil
		}
		// Firefox keeps a default size for each language group, chosen from the page's lang attribute.
		// x-western is Latin script and x-unicode is what a page with no language gets.
		prefs := make(map[string]any, 2*len(firefoxFontGroups))
		for _, g := range firefoxFontGroups {
			prefs["font.size.variable."+g] = variable
			prefs["font.size.monospace."+g] = fixed
		}
		return nil, prefs, nil
	}
	return nil, nil, fmt.Errorf("browser %q: want chrome or firefox", browser)
}

// firefoxFontGroups are the language groups of Firefox's font preferences. A page's lang attribute picks one.
var firefoxFontGroups = []string{
	"x-western", "x-unicode", "x-cyrillic", "x-central-euro", "x-baltic", "el", "tr", "he", "ar", "th",
	"ja", "ko", "zh-CN", "zh-HK", "zh-TW", "x-armn", "x-beng", "x-cans", "x-devanagari", "x-ethi",
	"x-geor", "x-gujr", "x-guru", "x-khmr", "x-knda", "x-mlym", "x-orya", "x-sinh", "x-tamil", "x-telu", "x-tibt",
}

func scaled(px, percent int) int { return (px*percent + 50) / 100 }

// WithTextScale returns a copy of opts with the text scale settings added for opts.Browser.
// Settings already in opts are kept unless the scale sets the same key.
func WithTextScale(opts LaunchOptions, percent int) (LaunchOptions, error) {
	chrome, firefox, err := TextScalePrefs(opts.Browser, percent)
	if err != nil {
		return opts, err
	}
	opts.ChromePrefs = mergePrefs(opts.ChromePrefs, chrome)
	opts.FirefoxPrefs = mergePrefs(opts.FirefoxPrefs, firefox)
	return opts, nil
}

func mergePrefs(base, extra map[string]any) map[string]any {
	if len(extra) == 0 {
		return base
	}
	out := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}
