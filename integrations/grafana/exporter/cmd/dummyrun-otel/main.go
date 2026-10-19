// Command dummyrun-otel exports Dummy Run run records to an OpenTelemetry backend such as
// Grafana's LGTM stack or Grafana Cloud. See the module README for usage.
package main

import (
	"context"
	"os"

	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/cli"
)

func main() {
	os.Exit(cli.Main(context.Background(), os.Args[1:], cli.Env{Stdout: os.Stdout, Stderr: os.Stderr}))
}
