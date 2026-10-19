package main

import (
	"encoding/base64"
	"html"

	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardlist"
	"github.com/grafana/grafana-foundation-sdk/go/text"
)

// Home is the local stack's home dashboard: the logo and a list of the Dummy Run dashboards.
func (c Config) Home() *dashboard.DashboardBuilder {
	ids := &ids{}
	b := dashboard.NewDashboardBuilder("Dummy Run: home").
		Uid(uidHome).
		Description("Start page for the local stack. It links to the four Dummy Run dashboards.").
		Tags([]string{"dummyrun-home"}).
		Editable().
		Time("now-14d", "now").
		Refresh("")

	content := `<div style="padding: 8px 4px;">`
	if c.LogoSVG != "" {
		src := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(c.LogoSVG))
		content += `<img src="` + src + `" alt="Dummy Run" height="64" style="display: block; margin-bottom: 12px;">`
	}
	content += `<p style="font-size: 16px; margin: 0;">` +
		html.EscapeString("AI personas have tried your app. These dashboards show what they did and what they found. Start with the overview, or open a persona to see one user's runs.") +
		`</p></div>`

	b.WithPanel(text.NewPanelBuilder().
		Id(ids.id()).
		Span(24).Height(5).
		Transparent(true).
		Mode(text.TextModeHTML).
		Content(content))
	b.WithPanel(dashboardlist.NewPanelBuilder().
		Id(ids.id()).
		Title("Dashboards").
		Span(24).Height(8).
		ShowSearch(true).
		ShowHeadings(false).
		ShowStarred(false).
		ShowRecentlyViewed(false).
		ShowFolderNames(false).
		KeepTime(true).
		IncludeVars(false).
		MaxItems(10).
		Tags([]string{tagDummyRun}))
	return b
}
