package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
)

// Dashboards returns every dashboard, keyed by its output file name.
func (c Config) Dashboards() map[string]*dashboard.DashboardBuilder {
	return map[string]*dashboard.DashboardBuilder{
		"overview.json": c.Overview(),
		"personas.json": c.Personas(),
		"findings.json": c.Findings(),
		"run.json":      c.Run(),
		"home.json":     c.Home(),
	}
}

// Render builds every dashboard and returns its JSON, keyed by file name.
func (c Config) Render() (map[string][]byte, error) {
	out := map[string][]byte{}
	for name, b := range c.Dashboards() {
		d, err := b.Build()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		raw, err := json.MarshalIndent(d, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		out[name] = append(raw, '\n')
	}
	return out, nil
}

func main() {
	var c Config
	out := flag.String("out", "json", "directory to write the dashboard JSON into")
	check := flag.Bool("check", false, "compare with the files in -out instead of writing, and fail if they differ")
	logo := flag.String("logo", "../../../brand/logo/lockup-horizontal-dark.svg", "SVG logo for the home dashboard, or empty for none")
	flag.StringVar(&c.PrometheusUID, "prometheus-uid", "prometheus", "data source UID for Prometheus or Mimir")
	flag.StringVar(&c.LokiUID, "loki-uid", "loki", "data source UID for Loki")
	flag.StringVar(&c.TempoUID, "tempo-uid", "tempo", "data source UID for Tempo")
	flag.Parse()

	if *logo != "" {
		svg, err := os.ReadFile(*logo)
		if err != nil {
			fail(err)
		}
		c.LogoSVG = string(svg)
	}
	files, err := c.Render()
	if err != nil {
		fail(err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}
	differ := 0
	for name, raw := range files {
		path := filepath.Join(*out, name)
		if *check {
			current, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(current, raw) {
				fmt.Fprintf(os.Stderr, "%s differs from the generator output\n", path)
				differ++
			}
			continue
		}
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			fail(err)
		}
		fmt.Println("wrote", path)
	}
	if differ > 0 {
		fmt.Fprintln(os.Stderr, "run `go run .` in integrations/grafana/dashboards and commit the result")
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
