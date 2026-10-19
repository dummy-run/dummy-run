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
	// newestSent is the newest log line time sent to the stream so far, seeded from Loki when known.
	newestSent time.Time
	// counts for the summary line
	runs, spans, logs, skippedLogs, presentLogs, presentRuns, metrics int
	created, existing                                                 int
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
	s.providers, err = otelsetup.New(ctx, otelsetup.Config{Protocol: proto, Temporality: temporality, Version: Version, Stderr: env.Stderr, Getenv: env.Getenv})
	if err != nil {
		return nil, err
	}
	s.emitter, err = telemetry.NewEmitter(s.providers.Tracer, s.providers.Meter, s.providers.Logger)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// logEntry is one log record with the run it belongs to.
type logEntry struct {
	runID string
	rec   telemetry.LogRecord
}

// prepare builds a run's plan and drops log records older than --logs-max-age.
func (s *session) prepare(run *record.Run) telemetry.Plan {
	p := telemetry.Build(run)
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
	sort.SliceStable(p.Logs, func(i, j int) bool { return p.Logs[i].Time.Before(p.Logs[j].Time) })
	return p
}

// lokiWindow is how far behind the newest line in a stream Loki still accepts a line
// (max_chunk_age / 2 with the default configuration).
const lokiWindow = time.Hour

// guard refuses log lines that Loki would drop: lines more than an hour older than the newest line
// already sent to this process's stream. Each process has its own stream, because the resource carries
// a per-process service.instance.id, so only this process's own lines count.
func (s *session) guard(oldest time.Time) error {
	if oldest.IsZero() || s.newestSent.IsZero() || !oldest.Before(s.newestSent.Add(-lokiWindow)) {
		return nil
	}
	return fmt.Errorf("log lines start at %s, more than an hour before the newest line already sent (%s), so Loki would drop them. Export runs in ended_at order, and never a batch that ends more than an hour before one already sent",
		oldest.UTC().Format(time.RFC3339), s.newestSent.UTC().Format(time.RFC3339))
}

// dropRunsAlreadyInLoki asks Loki which of the plans' runs already have all their log lines and marks
// those runs as exported, so a repeated export sends them again neither to Loki nor to Tempo. It
// does nothing without --verify-loki, and it cannot see runs whose log records --logs-max-age removed.
func (s *session) dropRunsAlreadyInLoki(ctx context.Context, plans []telemetry.Plan) (map[string]bool, error) {
	done := map[string]bool{}
	if s.loki == nil {
		return done, nil
	}
	var oldest, newest time.Time
	ids := make([]string, 0, len(plans))
	for _, p := range plans {
		if len(p.Logs) == 0 {
			continue
		}
		ids = append(ids, p.RunID)
		for _, l := range p.Logs {
			if oldest.IsZero() || l.Time.Before(oldest) {
				oldest = l.Time
			}
			if l.Time.After(newest) {
				newest = l.Time
			}
		}
	}
	if len(ids) == 0 {
		return done, nil
	}
	have, err := s.loki.Count(ctx, "dummyrun", ids, oldest, newest)
	if err != nil {
		return nil, fmt.Errorf("loki check: %w", err)
	}
	for _, p := range plans {
		if n := len(p.Logs); n > 0 && have[p.RunID] >= n {
			done[p.RunID] = true
			s.presentRuns++
			s.presentLogs += n
		}
	}
	return done, nil
}

// sendPlan emits one run's trace and metrics, its logs when withLogs is set, and posts its annotation.
func (s *session) sendPlan(ctx context.Context, path string, p telemetry.Plan, withLogs bool) error {
	s.emitter.EmitTrace(ctx, p)
	if err := s.emitter.EmitMetrics(ctx, p); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if withLogs {
		s.emitter.EmitLogs(ctx, p.RunID, p.Logs)
		s.noteSent(p.Logs)
	}
	s.runs++
	s.spans += 1 + len(p.Steps)
	s.logs += len(p.Logs)
	s.metrics += len(p.Metrics)
	if len(p.Logs) > 0 {
		s.wants = append(s.wants, verify.Want{RunID: p.RunID, Lines: len(p.Logs)})
	}
	return s.annotate(ctx, path, p)
}

// annotate posts the run's annotation unless Grafana already has it.
func (s *session) annotate(ctx context.Context, path string, p telemetry.Plan) error {
	if s.grafana == nil {
		return nil
	}
	res, err := s.grafana.Post(ctx, p.Annotation)
	if err != nil {
		return fmt.Errorf("%s: annotation: %w", path, err)
	}
	if res.Created {
		s.created++
	} else {
		s.existing++
	}
	return nil
}

// noteSent records the time span of log records sent, for the guard and the Loki check.
func (s *session) noteSent(logs []telemetry.LogRecord) {
	for _, l := range logs {
		if s.oldest.IsZero() || l.Time.Before(s.oldest) {
			s.oldest = l.Time
		}
		if l.Time.After(s.newest) {
			s.newest = l.Time
		}
		if l.Time.After(s.newestSent) {
			s.newestSent = l.Time
		}
	}
}

// sortedLogs flattens the plans' log records into one list, oldest first. Loki keeps one stream for
// the exporter and rejects a line more than an hour older than the newest line it already holds, so
// a batch of history has to arrive in time order.
func sortedLogs(plans []telemetry.Plan) []logEntry {
	var all []logEntry
	for _, p := range plans {
		for _, l := range p.Logs {
			all = append(all, logEntry{runID: p.RunID, rec: l})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].rec.Time.Before(all[j].rec.Time) })
	return all
}

// emitLogs sends sorted entries one at a time, each with its own run's trace context.
func (s *session) emitLogs(ctx context.Context, entries []logEntry) {
	for _, e := range entries {
		s.emitter.EmitLogs(ctx, e.runID, []telemetry.LogRecord{e.rec})
		s.noteSent([]telemetry.LogRecord{e.rec})
	}
}

func (s *session) summary() string {
	msg := fmt.Sprintf("sent %d runs: %d spans, %d log records, %d metric points", s.runs, s.spans, s.logs, s.metrics)
	if s.skippedLogs > 0 {
		msg += fmt.Sprintf("; skipped %d log records older than %s (see --logs-max-age)", s.skippedLogs, s.opts.logsMaxAge)
	}
	if s.presentRuns > 0 {
		msg += fmt.Sprintf("; %d runs with %d log records already in Loki, not sent again", s.presentRuns, s.presentLogs)
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
	plans := make([]telemetry.Plan, 0, len(files))
	for _, f := range files {
		plans = append(plans, s.prepare(f.Run))
	}
	done, err := s.dropRunsAlreadyInLoki(ctx, plans)
	if err != nil {
		fmt.Fprintf(env.Stderr, "dummyrun-otel: %v\n", err)
		return ExitFailure
	}
	fresh := make([]telemetry.Plan, 0, len(plans))
	freshPaths := make([]string, 0, len(plans))
	for i, p := range plans {
		if done[p.RunID] {
			// The annotation lookup is idempotent on its own, so it still runs for these.
			if err := s.annotate(ctx, files[i].Path, p); err != nil {
				fmt.Fprintf(env.Stderr, "dummyrun-otel: %v\n", err)
				return ExitFailure
			}
			continue
		}
		fresh = append(fresh, p)
		freshPaths = append(freshPaths, files[i].Path)
	}
	entries := sortedLogs(fresh)
	if len(entries) > 0 {
		if err := s.guard(entries[0].rec.Time); err != nil {
			fmt.Fprintf(env.Stderr, "dummyrun-otel: nothing sent: %v\n", err)
			return ExitFailure
		}
	}
	code := ExitOK
	for i, p := range fresh {
		if err := s.sendPlan(ctx, freshPaths[i], p, false); err != nil {
			fmt.Fprintf(env.Stderr, "dummyrun-otel: %v\n", err)
			code = ExitFailure
			break
		}
	}
	if code == ExitOK {
		s.emitLogs(ctx, entries)
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), opts.timeout)
	defer cancel()
	if err := s.providers.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(env.Stderr, "dummyrun-otel: export failed: %v\n", err)
		code = ExitFailure
	}
	fmt.Fprintln(env.Stderr, s.summary())
	if s.loki == nil && s.runs > 0 {
		fmt.Fprintln(env.Stderr, "note: without --verify-loki a repeated export of these runs writes their spans, log lines and metric points again; Tempo and Loki keep the duplicates")
	}
	if code == ExitOK && s.loki != nil {
		// The check gets its own deadline, so a slow flush does not eat into it.
		verifyCtx, cancelVerify := context.WithTimeout(context.WithoutCancel(ctx), verifyWait+opts.timeout)
		defer cancelVerify()
		if !s.verifyLogs(verifyCtx, s.wants, s.oldest, s.newest) {
			code = ExitFailure
		}
	}
	return code
}

// verifyWait is how long the Loki check waits for lines to appear, because the collector batches.
const verifyWait = 15 * time.Second

// verifyLogs counts the given runs' log lines back from Loki and reports every run that is short.
// It returns false when lines are missing or Loki could not be queried.
func (s *session) verifyLogs(ctx context.Context, wants []verify.Want, oldest, newest time.Time) bool {
	if len(wants) == 0 {
		if s.presentRuns > 0 {
			fmt.Fprintf(s.env.Stderr, "loki check: all %d runs were already in Loki, nothing to verify\n", s.presentRuns)
		} else {
			fmt.Fprintln(s.env.Stderr, "loki check: no log records were sent, nothing to verify")
		}
		return true
	}
	short, err := s.loki.Check(ctx, "dummyrun", wants, oldest, newest, verifyWait)
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
	fmt.Fprintf(s.env.Stderr, "dummyrun-otel: loki check: %d of %d runs are missing log lines. Loki drops a line more than an hour older than the newest line in its stream. Export runs in ended_at order, and never a batch that ends more than an hour before one already sent\n", len(short), len(wants))
	return false
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
		p := s.prepare(run)
		done, err := s.dropRunsAlreadyInLoki(ctx, []telemetry.Plan{p})
		if err != nil {
			fmt.Fprintf(env.Stderr, "dummyrun-otel: %s: %v\n", path, err)
			failed++
			return
		}
		if done[p.RunID] {
			if err := s.annotate(ctx, path, p); err != nil {
				fmt.Fprintf(env.Stderr, "dummyrun-otel: %v\n", err)
				failed++
				return
			}
			fmt.Fprintf(env.Stderr, "already in Loki, not sent again: %s\n", path)
			return
		}
		if len(p.Logs) > 0 {
			if err := s.guard(p.Logs[0].Time); err != nil {
				fmt.Fprintf(env.Stderr, "dummyrun-otel: %s: not sent: %v\n", path, err)
				failed++
				return
			}
		}
		if err := s.sendPlan(ctx, path, p, true); err != nil {
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
		if s.loki != nil && len(p.Logs) > 0 {
			verifyCtx, cancelVerify := context.WithTimeout(ctx, verifyWait+opts.timeout)
			defer cancelVerify()
			if !s.verifyLogs(verifyCtx, []verify.Want{{RunID: p.RunID, Lines: len(p.Logs)}}, p.Logs[0].Time, p.Logs[len(p.Logs)-1].Time) {
				fmt.Fprintf(env.Stderr, "dummyrun-otel: %s: log lines did not reach Loki\n", path)
				failed++
				return
			}
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
