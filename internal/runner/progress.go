package runner

// ProgressReporter receives progress events as Run executes, so a caller
// (typically the CLI) can show the operator something is happening across
// a run that may issue hundreds of requests at 30-50s each. Implementations
// must be safe for concurrent use: ReportDone is called from up to
// Config.Concurrency goroutines at once.
type ProgressReporter interface {
	// ReportStart is called once, before any request is issued.
	// totalTests and totalModels are the catalog and model-list sizes
	// being fanned out (totalTests*totalModels is the total request
	// count); concurrency is the bound Run is applying.
	ReportStart(totalTests, totalModels, concurrency int)

	// ReportDone is called once per completed (model, test) call, after
	// Run has already recorded result into its output slice. done is this
	// call's 1-based completion index; total is the overall request
	// count (totalTests*totalModels from ReportStart).
	ReportDone(done, total int, result Result)
}

// NoopProgressReporter discards every event. It is the default Runner uses
// when Config.Reporter is nil (unit tests, and any caller that does not
// want progress output, e.g. the CLI's --quiet flag).
type NoopProgressReporter struct{}

// ReportStart implements ProgressReporter.
func (NoopProgressReporter) ReportStart(_, _, _ int) {}

// ReportDone implements ProgressReporter.
func (NoopProgressReporter) ReportDone(_, _ int, _ Result) {}
