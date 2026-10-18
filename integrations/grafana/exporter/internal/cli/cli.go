// Package cli implements the dummyrun-otel command.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/annotations"
	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/otelsetup"
	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/record"
	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/telemetry"
	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/verify"
	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/watch"
)

// Exit codes.
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
)

// Version is set by the build with -ldflags "-X ...cli.Version=v1.2.3".
var Version = "dev"

// Env is the process environment the command reads. Tests substitute their own.
type Env struct {
	Stdout io.Writer
	Stderr io.Writer
	Getenv func(string) string
	// Now exists so tests can fix the clock the log age check uses.
	Now func() time.Time
	// Signals, when nil, means watch stops on SIGINT or SIGTERM.
	Signals <-chan os.Signal
}

// Main runs the command with args (without the program name) and returns the exit code.
func Main(ctx context.Context, args []string, env Env) int {
	if env.Getenv == nil {
		env.Getenv = os.Getenv
	}
	if env.Now == nil {
		env.Now = time.Now
	}
	if len(args) == 0 {
		usage(env.Stderr)
		return ExitUsage
	}
	switch args[0] {
	case "export":
		return runExport(ctx, args[1:], env)
	case "watch":
		return runWatch(ctx, args[1:], env)
	case "version", "--version", "-v":
		fmt.Fprintf(env.Stdout, "dummyrun-otel %s\n", Version)
		return ExitOK
	case "help", "-h", "--help":
		usage(env.Stdout)
		return ExitOK
	default:
		fmt.Fprintf(env.Stderr, "dummyrun-otel: unknown command %q\n\n", args[0])
		usage(env.Stderr)
		return ExitUsage
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  dummyrun-otel export [flags] FILE|DIR...   send run records to an OTLP endpoint
  dummyrun-otel watch  [flags] DIR           send new run records as they appear
  dummyrun-otel version

The OTLP endpoint, headers and TLS settings come from the standard variables:
OTEL_EXPORTER_OTLP_ENDPOINT, OTEL_EXPORTER_OTLP_HEADERS, OTEL_EXPORTER_OTLP_PROTOCOL,
OTEL_RESOURCE_ATTRIBUTES. Grafana annotations need --grafana-url and GRAFANA_TOKEN.
Run "dummyrun-otel export -h" for the flags.
`)
}

// options are the flags shared by export and watch.
type options struct {
	dryRun      bool
	protocol    string
	temporality string
	grafanaURL  string
	verifyLoki  string
	logsMaxAge  time.Duration
	timeout     time.Duration
}

func (o *options) bind(fs *flag.FlagSet, env Env) {
	fs.BoolVar(&o.dryRun, "dry-run", false, "print what would be sent as JSON and send nothing")
	fs.StringVar(&o.protocol, "protocol", env.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"), "OTLP transport: http/protobuf (default) or grpc")
	fs.StringVar(&o.temporality, "metrics-temporality", "cumulative", "metric temporality: cumulative or delta")
	fs.StringVar(&o.grafanaURL, "grafana-url", "", "Grafana base URL for annotations; the token comes from GRAFANA_TOKEN")
	fs.StringVar(&o.verifyLoki, "verify-loki", "", "Loki base URL; after the export, count the log lines back per run and fail when any are missing. Basic auth comes from LOKI_BASIC_AUTH as user:password")
	fs.DurationVar(&o.logsMaxAge, "logs-max-age", 168*time.Hour, "skip log records older than this, because Loki rejects them by default; 0 sends every record")
	fs.DurationVar(&o.timeout, "timeout", otelsetup.Timeout, "how long to wait for the final flush")
}

type session struct {
	env       Env
	opts      options
	providers *otelsetup.Providers
	emitter   *telemetry.Emitter
	grafana   *annotations.Client
	loki      *verify.Loki
	// wants is what the Loki check expects per run, with the time span of the records sent.
	wants          []verify.Want
	oldest, newest time.Time
	// counts for the summary line
	runs, spans, logs, skippedLogs, metrics int
	created, existing                       int
}

func newSession(ctx context.Context, opts options, env Env) (*session, error) {
	proto, err := otelsetup.ParseProtocol(opts.protocol)
	if err != nil {
		return nil, err
	}
	temporality, err := otelsetup.ParseTemporality(opts.temporality)
	if err != nil {
		return nil, err
	}
	s := &session{env: env, opts: opts}
	if opts.grafanaURL != "" {
		s.grafana, err = annotations.New(opts.grafanaURL, env.Getenv("GRAFANA_TOKEN"), nil)
		if err != nil {
			return nil, err
		}
	}
	if opts.verifyLoki != "" {
		s.loki, err = verify.NewLoki(opts.verifyLoki, env.Getenv("LOKI_BASIC_AUTH"), nil)
		if err != nil {
			return nil, err
		}
	}
	s.providers, err = otelsetup.New(ctx, otelsetup.Config{Protocol: proto, Temporality: temporality, Version: Version, Stderr: env.Stderr})
	if err != nil {
		return nil, err
	}
	s.emitter, err = telemetry.NewEmitter(s.providers.Tracer, s.providers.Meter, s.providers.Logger)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// runLogs are one run's log records, kept aside so export can send every run's logs in time order.
type runLogs struct {
	runID string
	logs  []telemetry.LogRecord
}

// send emits one run and posts its annotation. When deferLogs is non-nil the log records are
// appended to it instead of being sent.
func (s *session) send(ctx context.Context, f record.File, deferLogs *[]runLogs) error {
	p := telemetry.Build(f.Run)
	if s.opts.logsMaxAge > 0 {
		cutoff := s.env.Now().Add(-s.opts.logsMaxAge)
		kept := p.Logs[:0]
		for _, l := range p.Logs {
			if l.Time.Before(cutoff) {
				s.skippedLogs++
				continue
			}
			kept = append(kept, l)
		}
		p.Logs = kept
	}
	s.emitter.EmitTrace(ctx, p)
	if err := s.emitter.EmitMetrics(ctx, p); err != nil {
		return fmt.Errorf("%s: %w", f.Path, err)
	}
	if deferLogs != nil {
		*deferLogs = append(*deferLogs, runLogs{runID: p.RunID, logs: p.Logs})
	} else {
		s.emitter.EmitLogs(ctx, p.RunID, p.Logs)
	}
	s.runs++
	s.spans += 1 + len(p.Steps)
	s.logs += len(p.Logs)
	s.metrics += len(p.Metrics)
	if len(p.Logs) > 0 {
		s.wants = append(s.wants, verify.Want{RunID: p.RunID, Lines: len(p.Logs)})
		for _, l := range p.Logs {
			if s.oldest.IsZero() || l.Time.Before(s.oldest) {
				s.oldest = l.Time
			}
			if l.Time.After(s.newest) {
				s.newest = l.Time
			}
		}
	}
	if s.grafana != nil {
		res, err := s.grafana.Post(ctx, p.Annotation)
		if err != nil {
			return fmt.Errorf("%s: annotation: %w", f.Path, err)
		}
		if res.Created {
			s.created++
		} else {
			s.existing++
		}
	}
	return nil
}

func (s *session) summary() string {
	msg := fmt.Sprintf("sent %d runs: %d spans, %d log records, %d metric points", s.runs, s.spans, s.logs, s.metrics)
	if s.skippedLogs > 0 {
		msg += fmt.Sprintf("; skipped %d log records older than %s (see --logs-max-age)", s.skippedLogs, s.opts.logsMaxAge)
	}
	if s.grafana != nil {
		msg += fmt.Sprintf("; annotations: %d created, %d already present", s.created, s.existing)
	}
	return msg
}

func runExport(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("dummyrun-otel export", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	var opts options
	opts.bind(fs, env)
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(env.Stderr, "dummyrun-otel export: give at least one run record file or directory")
		return ExitUsage
	}

	validator, err := record.NewValidator()
	if err != nil {
		fmt.Fprintf(env.Stderr, "dummyrun-otel: %v\n", err)
		return ExitFailure
	}
	files, err := validator.Load(fs.Args()...)
	if err != nil {
		fmt.Fprintf(env.Stderr, "dummyrun-otel: %v\n", err)
		return ExitFailure
	}

	if opts.dryRun {
		plans := make([]telemetry.Plan, 0, len(files))
		for _, f := range files {
			plans = append(plans, telemetry.Build(f.Run))
		}
		enc := json.NewEncoder(env.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(plans); err != nil {
			fmt.Fprintf(env.Stderr, "dummyrun-otel: %v\n", err)
			return ExitFailure
		}
		fmt.Fprintf(env.Stderr, "dry run: %d runs read and validated, nothing sent\n", len(files))
		return ExitOK
	}

	s, err := newSession(ctx, opts, env)
	if err != nil {
		fmt.Fprintf(env.Stderr, "dummyrun-otel: %v\n", err)
		return ExitFailure
	}
	code := ExitOK
	var deferred []runLogs
	for _, f := range files {
		if err := s.send(ctx, f, &deferred); err != nil {
			fmt.Fprintf(env.Stderr, "dummyrun-otel: %v\n", err)
			code = ExitFailure
			break
		}
	}
	if code == ExitOK {
		s.emitSortedLogs(ctx, deferred)
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), opts.timeout)
	defer cancel()
	if err := s.providers.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(env.Stderr, "dummyrun-otel: export failed: %v\n", err)
		code = ExitFailure
	}
	fmt.Fprintln(env.Stderr, s.summary())
	if code == ExitOK && s.loki != nil {
		if !s.verifyLogs(shutdownCtx) {
			code = ExitFailure
		}
	}
	return code
}

// verifyLogs counts the exported runs' log lines back from Loki and reports every run that is short.
// It returns false when lines are missing or Loki could not be queried.
func (s *session) verifyLogs(ctx context.Context) bool {
	if len(s.wants) == 0 {
		fmt.Fprintln(s.env.Stderr, "loki check: no log records were sent, nothing to verify")
		return true
	}
	short, err := s.loki.Check(ctx, "dummyrun", s.wants, s.oldest, s.newest, 15*time.Second)
	if err != nil {
		fmt.Fprintf(s.env.Stderr, "dummyrun-otel: loki check: %v\n", err)
		return false
	}
	if len(short) == 0 {
		fmt.Fprintf(s.env.Stderr, "loki check: every one of the %d log records is in Loki\n", s.logs)
		return true
	}
	for _, sf := range short {
		fmt.Fprintf(s.env.Stderr, "dummyrun-otel: %s\n", sf)
	}
	fmt.Fprintf(s.env.Stderr, "dummyrun-otel: loki check: %d of %d runs are missing log lines. Loki drops a line more than an hour older than the newest line in its stream. Export runs in ended_at order, and never a batch that ends more than an hour before one already sent\n", len(short), len(s.wants))
	return false
}

// emitSortedLogs sends every run's log records oldest first. Loki keeps one stream for the exporter
// and rejects a line more than an hour older than the newest line it already holds, so a batch of
// history has to arrive in time order.
func (s *session) emitSortedLogs(ctx context.Context, runs []runLogs) {
	type entry struct {
		runID string
		rec   telemetry.LogRecord
	}
	var all []entry
	for _, r := range runs {
		for _, l := range r.logs {
			all = append(all, entry{runID: r.runID, rec: l})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].rec.Time.Before(all[j].rec.Time) })
	for _, e := range all {
		s.emitter.EmitLogs(ctx, e.runID, []telemetry.LogRecord{e.rec})
	}
}

func runWatch(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("dummyrun-otel watch", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	var opts options
	opts.bind(fs, env)
	interval := fs.Duration("interval", 5*time.Second, "how often to look for new files")
	existing := fs.Bool("existing", false, "also send the records already in the directory at start")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(env.Stderr, "dummyrun-otel watch: give exactly one directory")
		return ExitUsage
	}
	dir := fs.Arg(0)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		fmt.Fprintf(env.Stderr, "dummyrun-otel watch: %s is not a directory\n", dir)
		return ExitUsage
	}
	if opts.dryRun {
		fmt.Fprintln(env.Stderr, "dummyrun-otel watch: --dry-run is only for export")
		return ExitUsage
	}

	validator, err := record.NewValidator()
	if err != nil {
		fmt.Fprintf(env.Stderr, "dummyrun-otel: %v\n", err)
		return ExitFailure
	}
	s, err := newSession(ctx, opts, env)
	if err != nil {
		fmt.Fprintf(env.Stderr, "dummyrun-otel: %v\n", err)
		return ExitFailure
	}

	ctx, stop := context.WithCancel(ctx)
	defer stop()
	sigs := env.Signals
	if sigs == nil {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(c)
		sigs = c
	}
	go func() {
		select {
		case <-sigs:
			stop()
		case <-ctx.Done():
		}
	}()

	failed := 0
	fmt.Fprintf(env.Stderr, "watching %s every %s\n", dir, *interval)
	poller := &watch.Poller{Dir: dir, Interval: *interval, IncludeExisting: *existing}
	_ = poller.Run(ctx, func(path string) {
		run, err := validator.LoadFile(path)
		if err != nil {
			fmt.Fprintf(env.Stderr, "dummyrun-otel: %v\n", err)
			failed++
			return
		}
		if err := s.send(ctx, record.File{Path: path, Run: run}, nil); err != nil {
			fmt.Fprintf(env.Stderr, "dummyrun-otel: %v\n", err)
			failed++
			return
		}
		flushCtx, cancel := context.WithTimeout(ctx, opts.timeout)
		defer cancel()
		if err := s.providers.Flush(flushCtx); err != nil {
			fmt.Fprintf(env.Stderr, "dummyrun-otel: %s: export failed: %v\n", path, err)
			failed++
			return
		}
		fmt.Fprintf(env.Stderr, "sent %s\n", path)
	})

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), opts.timeout)
	defer cancel()
	code := ExitOK
	if err := s.providers.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(env.Stderr, "dummyrun-otel: export failed: %v\n", err)
		code = ExitFailure
	}
	if failed > 0 {
		fmt.Fprintf(env.Stderr, "%d files failed\n", failed)
		code = ExitFailure
	}
	fmt.Fprintln(env.Stderr, s.summary())
	return code
}
