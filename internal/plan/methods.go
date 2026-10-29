package plan

import (
	"fmt"
	"strings"
)

// Method is how one persona goes about any journey: its opening moves, how it tries each journey, what it
// watches for and what it reports. It belongs to the persona, keyed by library id, so the plan only chooses
// the journeys, the pages and the settings. Personas without one get genericMethod.
type Method struct {
	// Budget is advice on spending steps, printed before the steps. It may use %[1]d for the step budget and
	// %[2]d for the steps per journey.
	Budget string
	// Open are the first steps, before any journey.
	Open []string
	// Journey is how the persona tries one journey: %[1]s is the journey's name, %[2]s where it starts.
	Journey string
	// Extra are steps after the journeys and before the accessibility check.
	Extra []string
	// Watch is what the persona notes on every page.
	Watch []string
	// Report is what goes in the persona's report. %[1]s is the persona's name.
	Report string
	// MinSteps is the minimum number of steps for this persona's way of working.
	MinSteps int
}

// genericWatch is what every persona without its own list notes. It is general on purpose and never mentions
// prices: the plan decides where the persona goes, not what counts as a problem.
var genericWatch = []string{
	"Links and buttons whose name does not say where they go or what they do, such as several links all called \"Read more\", or controls shown as \"(no label)\".",
	"Form fields with no visible label, and groups of choices with no question above them.",
	"Images that carry meaning but have no description, and pages with no headings or with headings in a strange order.",
	"Text that is small, faint or cut off, and content that overlaps or runs off the screen.",
	"Links that go somewhere you did not expect, pages that fail to load or answer with an error, and pages that are very slow.",
	"Words or instructions you do not understand, and a next step you cannot find.",
}

var genericMethod = Method{
	Open:     []string{"Read the first screen. Write down in one sentence what you think the site is and who it is for."},
	Journey:  "%[1]s.%[2]s Read the headings and the buttons, and scroll when the top of the page is not enough.",
	Watch:    genericWatch,
	Report:   "%[1]s's one-sentence guess at what the site is and whether the site proved it right. For each journey, whether it worked. Anything missing, broken or confusing, with step numbers and the control or the words on the page.",
	MinSteps: 6,
}

// Methods are the personas' own ways of working. The keyboard and screen-reader methods come from the hosted
// runner's tuned scenarios: a key press is a step, so the keyboard user counts Tab presses and plans them.
var Methods = map[string]Method{
	"first-time-visitor": genericMethod,
	"keyboard-only-user": {
		Budget: "Each key press is one step, so spend them where they count. You have %[1]d; give each journey about %[2]d. " +
			"Use the list of controls on each page to work out how many Tab presses a control should take, press Tab only as far as you need to reach it, " +
			"and do not Tab through the footer or parts of the page that do not matter.",
		Open: []string{"Press Tab from the top of the page. Note the first stop and whether a skip link shows. Use it if there is one."},
		Journey: "%[1]s, with the keyboard only.%[2]s Tab to the control that starts it, count the Tab presses, and note whether focus is visible at each stop. " +
			"Press Enter to follow a link. If a menu opens, use Enter or the arrow keys, close it with Escape and check that focus goes back to where it was. Stop before any form is sent.",
		Extra: []string{"If a banner, dialog or pop-up appears at any point, Tab round it and leave it with the keyboard."},
		Watch: []string{
			"A control you cannot reach with Tab, or that Tab reaches in a strange order.",
			"Focus you cannot see, or that disappears behind something.",
			"No skip link, or a long menu to get through before the content.",
			"A menu, pop-up or widget you cannot leave with the keyboard, and focus that does not return to where it was.",
			"Links and buttons shown as \"(no label)\", or named \"click here\" or \"more\".",
			"A form field with no label, or a control that does nothing when you press Enter or Space.",
		},
		Report:   "The path as steps and Tab counts. Stops where focus was hidden, where Tab skipped a control or where %[1]s got stuck, with step numbers. Which journeys %[1]s could not finish with the keyboard.",
		MinSteps: 12,
	},
	"screen-reader-user": {
		Open: []string{
			"Read the page title and the main heading. Say what the page is for.",
			"List the headings in order and judge whether the outline makes sense. Note whether there is a skip link and a main landmark.",
		},
		Journey: "%[1]s, using only headings, landmarks and the names of links.%[2]s Judge whether each link name says where it goes when heard on its own, " +
			"and check the title, the headings and the main landmark of each page you reach.",
		Watch: []string{
			"Headings that are missing, skip a level or say nothing about their section.",
			"No main landmark, no skip link, or the same page title on every page.",
			"Links and buttons shown as \"(no name)\", or named \"click here\" or \"more\".",
			"Images, icons and charts with no description.",
			"Form fields with no label, and errors or changes on the page that are not announced.",
			"Something you could only find by looking at the page.",
		},
		Report:   "The path as steps. Headings that were missing, skipped a level or said nothing. Links that did not say where they go. The accessibility check's rule IDs, with step numbers.",
		MinSteps: 10,
	},
	"low-vision-zoom": {
		Open:    []string{"Read the first screen at your zoom. Say what the site is and whether anything is cut off or covered."},
		Journey: "%[1]s.%[2]s After each step look at the screenshot for text that is cut off, controls that overlap, bars that cover the page and anything that needs sideways scrolling.",
		Watch: append([]string{
			"Content that needs sideways scrolling, or text cut off at the edge of the window.",
			"Sticky headers, banners or chat buttons that cover what you are reading.",
		}, genericWatch...),
		Report:   genericMethod.Report,
		MinSteps: 6,
	},
	"large-text-phone": {
		Open:    []string{"Read the first screen with your large text. Say what the site is and whether any words or buttons are clipped."},
		Journey: "%[1]s.%[2]s After each step look at the screenshot and the layout line for overlapping or clipped controls and text that runs off the screen.",
		Watch: append([]string{
			"Buttons and labels that overlap or are clipped when the text is larger.",
			"Text that runs off the screen or under another element.",
		}, genericWatch...),
		Report:   genericMethod.Report,
		MinSteps: 6,
	},
}

// methodFor returns the persona's own method, or the generic one.
func methodFor(id string) Method {
	if m, ok := Methods[id]; ok {
		return m
	}
	return genericMethod
}

// compose writes the persona's steps and minimum for its journeys. minSteps of 0 uses the persona's own.
func compose(p *Plan, pick *Pick, steps, minSteps int) {
	m := methodFor(pick.ID)
	names := map[string]Journey{}
	for _, j := range p.Journeys {
		names[j.ID] = j
	}
	var out []string
	out = append(out, m.Open...)
	for _, id := range pick.Journeys {
		j, ok := names[id]
		if !ok {
			continue
		}
		where := ""
		if len(j.StartPages) > 0 {
			where = " Start from " + strings.Join(j.StartPages, " or ") + "."
		}
		out = append(out, fmt.Sprintf(m.Journey, strings.TrimSuffix(j.Name, "."), where))
	}
	out = append(out, m.Extra...)
	pick.Scenario.Steps = out
	if minSteps <= 0 {
		minSteps = m.MinSteps
	}
	pick.Scenario.MinSteps = clampMin(minSteps, steps)
}

// budgetText is the persona's advice on spending steps, or "".
func budgetText(id string, steps, journeys int) string {
	m := methodFor(id)
	if m.Budget == "" {
		if journeys > 1 {
			return fmt.Sprintf("You have %d actions. Give each journey about %d, so you reach them all.", steps, max(steps/journeys, 1))
		}
		return ""
	}
	return fmt.Sprintf(m.Budget, steps, max(steps/max(journeys, 1), 1))
}
