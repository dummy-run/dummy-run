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
	"strings"
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
	// ExitAnnotations means every trace, metric and log record was sent and flushed, but some
	// Grafana annotations could not be created. A re-export with --signals annotations fills them in.
	ExitAnnotations = 3
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
	// Sleep, when set, replaces the waits between annotation calls and retries. Tests use it.
	Sleep func(context.Context, time.Duration) error
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
	signals     string
	// signalsSet is true when the user passed --signals, so the default is told apart from a choice.
	signalsSet bool
	verifyLoki string
	logsMaxAge time.Duration
	timeout    time.Duration
}

func (o *options) bind(fs *flag.FlagSet, env Env) {
	fs.BoolVar(&o.dryRun, "dry-run", false, "print what would be sent as JSON and send nothing")
	fs.StringVar(&o.protocol, "protocol", env.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"), "OTLP transport: http/protobuf (default) or grpc")
	fs.StringVar(&o.temporality, "metrics-temporality", "cumulative", "metric temporality: cumulative or delta")
	fs.StringVar(&o.grafanaURL, "grafana-url", "", "Grafana base URL for annotations; the token comes from GRAFANA_TOKEN")
	fs.StringVar(&o.signals, "signals", "traces,metrics,logs,annotations", "what to send, a comma separated list of traces, metrics, logs and annotations; use it to send only what an earlier export missed. Annotations also need --grafana-url")
	fs.StringVar(&o.verifyLoki, "verify-loki", "", "Loki base URL; after the export, count the log lines back per run and fail when any are missing. Basic auth comes from LOKI_BASIC_AUTH as user:password")
	fs.DurationVar(&o.logsMaxAge, "logs-max-age", 168*time.Hour, "skip log records older than this, because Loki rejects them by default; 0 sends every record")
	fs.DurationVar(&o.timeout, "timeout", otelsetup.Timeout, "how long to wait for the final flush")
}

// signalSet is the signals an export sends.
type signalSet struct {
	traces, metrics, logs, annotations bool
}

// parseSignals reads the --signals value.
func parseSignals(v string) (signalSet, error) {
	var set signalSet
	for _, name := range strings.Split(v, ",") {
		switch strings.TrimSpace(name) {
		case "traces":
			set.traces = true
		case "metrics":
			set.metrics = true
		case "logs":
			set.logs = true
		case "annotations":
			set.annotations = true
		default:
			return signalSet{}, fmt.Errorf("--signals: unknown signal %q; use traces, metrics, logs and annotations", name)
		}
	}
	return set, nil
}

// check validates flag combinations that the flag package cannot.
func (o *options) check() (signalSet, error) {
	sig, err := parseSignals(o.signals)
	if err != nil {
		return sig, err
	}
	if sig.annotations && o.grafanaURL == "" && (o.signalsSet || sig == (signalSet{annotations: true})) {
		return sig, errors.New("--signals annotations needs --grafana-url")
	}
	if o.verifyLoki != "" && !sig.logs {
		return sig, errors.New("--verify-loki checks log lines, so --signals must include logs")
	}
	return sig, nil
}

type session struct {
	env  Env
	opts options
	sig  signalSet
	// annotationFailures holds one message for each annotation that could not be created.
	annotationFailures []string
	// annotateDeadline ends the time all annotation calls share. It is set at the first annotation.
	annotateDeadline time.Time
	providers        *otelsetup.Providers
	emitter          *telemetry.Emitter
	grafana          *annotations.Client
	loki             *verify.Loki
	// wants is what the Loki check expects per run, with the time span of the records sent.
	wants          []verify.Want
	oldest, newest time.Time
	// newestSent is the newest log line time sent to the stream so far, seeded from Loki when known.
	newestSent time.Time
	// counts for the summary line
	runs, spans, logs, skippedLogs, presentLogs, presentRuns, metrics int
	created, existing                                                 int
}

func newSession(ctx context.Context, opts options, sig signalSet, env Env) (*session, error) {
	proto, err := otelsetup.ParseProtocol(opts.protocol)
	if err != nil {
		return nil, err
	}
	temporality, err := otelsetup.ParseTemporality(opts.temporality)
	if err != nil {
		return nil, err
	}
	s := &session{env: env, opts: opts, sig: sig}
	if opts.grafanaURL != "" && sig.annotations {
		s.grafana, err = annotations.New(opts.grafanaURL, env.Getenv("GRAFANA_TOKEN"), nil)
		if err != nil {
			return nil, err
		}
		if env.Sleep != nil {
			s.grafana.Sleep = env.Sleep
		}
	}
	if opts.verifyLoki != "" {
		s.loki, err = verify.NewLoki(opts.verifyLoki, env.Getenv("LOKI_BASIC_AUTH"), nil)
		if err != nil {
			return nil, err
		}
	}
	if !sig.traces && !sig.metrics && !sig.logs {
		// Annotations only: no OTLP connection, so nothing else is sent, not even an empty batch.
		return s, nil
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

// shutdown flushes and releases the OTLP providers, if the session has any.
func (s *session) shutdown(ctx context.Context) error {
	if s.providers == nil {
		return nil
	}
	return s.providers.Shutdown(ctx)
}

// flush pushes what is queued, if the session has OTLP providers.
func (s *session) flush(ctx context.Context) error {
	if s.providers == nil {
		return nil
	}
	return s.providers.Flush(ctx)
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
	// The annotation goes first, so a metrics error does not skip it. A failed annotation is only
	// noted, and it never stops the telemetry.
	s.annotate(ctx, path, p)
	s.runs++
	if s.sig.traces {
		s.emitter.EmitTrace(ctx, p)
		s.spans += 1 + len(p.Steps)
	}
	if s.sig.logs {
		if withLogs {
			s.emitter.EmitLogs(ctx, p.RunID, p.Logs)
			s.noteSent(p.Logs)
		}
		s.logs += len(p.Logs)
		if len(p.Logs) > 0 {
			s.wants = append(s.wants, verify.Want{RunID: p.RunID, Lines: len(p.Logs)})
		}
	}
	if s.sig.metrics {
		if err := s.emitter.EmitMetrics(ctx, p); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		s.metrics += len(p.Metrics)
	}
	return nil
}

// annotationBudget is the most time one export spends on annotation calls, waits included.
// It is a variable so a test can shorten it.
var annotationBudget = 10 * time.Minute

// annotate posts the run's annotation unless Grafana already has it. A failure is printed and
// kept for the report at the end, and the caller carries on.
func (s *session) annotate(ctx context.Context, path string, p telemetry.Plan) {
	if s.grafana == nil {
		return
	}
	// The annotations share one budget, apart from the telemetry, so a long run of refusals cannot
	// hold up the traces, metrics and logs. The telemetry uses the caller's context and has no part in it.
	if s.annotateDeadline.IsZero() {
		s.annotateDeadline = time.Now().Add(annotationBudget)
	}
	actx, cancel := context.WithDeadline(ctx, s.annotateDeadline)
	defer cancel()
	var res annotations.Result
	var err error
	if time.Now().After(s.annotateDeadline) {
		err = fmt.Errorf("the annotation time budget of %s is used up", annotationBudget)
	} else {
		res, err = s.grafana.Post(actx, p.Annotation)
	}
	if err != nil {
		msg := fmt.Sprintf("%s: annotation: %v", path, err)
		fmt.Fprintf(s.env.Stderr, "dummyrun-otel: %s\n", msg)
		s.annotationFailures = append(s.annotationFailures, msg)
		return
	}
	if res.Created {
		s.created++
	} else {
		s.existing++
	}
}

// reportAnnotations lists the annotations that failed. It prints nothing when none did.
func (s *session) reportAnnotations() {
	if len(s.annotationFailures) == 0 {
		return
	}
	fmt.Fprintf(s.env.Stderr, "dummyrun-otel: %d annotations could not be created. The telemetry was sent. Run the export again with --signals annotations to add them:\n", len(s.annotationFailures))
	for _, f := range s.annotationFailures {
		fmt.Fprintf(s.env.Stderr, "  %s\n", f)
	}
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
		if n := len(s.annotationFailures); n > 0 {
			msg += fmt.Sprintf(", %d failed", n)
		}
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
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "signals" {
			opts.signalsSet = true
		}
	})
	sig, err := opts.check()
	if err != nil {
		fmt.Fprintf(env.Stderr, "dummyrun-otel export: %v\n", err)
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

	s, err := newSession(ctx, opts, sig, env)
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
			s.annotate(ctx, files[i].Path, p)
			continue
		}
		fresh = append(fresh, p)
		freshPaths = append(freshPaths, files[i].Path)
	}
	var entries []logEntry
	if sig.logs {
		entries = sortedLogs(fresh)
	}
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
		}
	}
	// The logs and the final flush happen whatever went wrong before.
	s.emitLogs(ctx, entries)
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), opts.timeout)
	defer cancel()
	if err := s.shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(env.Stderr, "dummyrun-otel: export failed: %v\n", err)
		code = ExitFailure
	}
	fmt.Fprintln(env.Stderr, s.summary())
	s.reportAnnotations()
	if s.loki == nil && s.runs > 0 && sig.logs {
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
	if code == ExitOK && len(s.annotationFailures) > 0 {
		code = ExitAnnotations
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
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "signals" {
			opts.signalsSet = true
		}
	})
	sig, err := opts.check()
	if err != nil {
		fmt.Fprintf(env.Stderr, "dummyrun-otel watch: %v\n", err)
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
	s, err := newSession(ctx, opts, sig, env)
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
		// Each new run record gets a fresh annotation budget, so a long watch keeps annotating.
		s.annotateDeadline = time.Time{}
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
			s.annotate(ctx, path, p)
			fmt.Fprintf(env.Stderr, "already in Loki, not sent again: %s\n", path)
			return
		}
		if sig.logs && len(p.Logs) > 0 {
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
		if err := s.flush(flushCtx); err != nil {
			fmt.Fprintf(env.Stderr, "dummyrun-otel: %s: export failed: %v\n", path, err)
			failed++
			return
		}
		if s.loki != nil && sig.logs && len(p.Logs) > 0 {
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
	if err := s.shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(env.Stderr, "dummyrun-otel: export failed: %v\n", err)
		code = ExitFailure
	}
	if failed > 0 {
		fmt.Fprintf(env.Stderr, "%d files failed\n", failed)
		code = ExitFailure
	}
	fmt.Fprintln(env.Stderr, s.summary())
	s.reportAnnotations()
	if code == ExitOK && len(s.annotationFailures) > 0 {
		code = ExitAnnotations
	}
	return code
}
