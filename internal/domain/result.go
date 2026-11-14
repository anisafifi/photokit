package domain

import (
	"encoding/json"
	"time"
)

// Status is the outcome of processing one file.
type Status string

// Statuses.
const (
	StatusProcessed Status = "processed"
	StatusSkipped   Status = "skipped"
	StatusFailed    Status = "failed"
	StatusPlanned   Status = "planned" // dry-run only
)

// Result describes what happened to one input file.
type Result struct {
	Source     string   `json:"source"`
	Dest       string   `json:"destination,omitempty"`
	Status     Status   `json:"status"`
	Operations []string `json:"operations,omitempty"`
	Reason     string   `json:"reason,omitempty"`
	Error      string   `json:"error,omitempty"`
}

// Report summarises a batch run.
type Report struct {
	Total     int
	Processed int
	Skipped   int
	Failed    int
	Planned   int
	DryRun    bool
	Duration  time.Duration
	Results   []Result
}

// Add records one result and updates the counters.
func (r *Report) Add(res Result) {
	r.Total++
	switch res.Status {
	case StatusProcessed:
		r.Processed++
	case StatusSkipped:
		r.Skipped++
	case StatusFailed:
		r.Failed++
	case StatusPlanned:
		r.Planned++
	}
	r.Results = append(r.Results, res)
}

// MarshalJSON renders the report in the documented JSON shape.
func (r Report) MarshalJSON() ([]byte, error) {
	type out struct {
		Total     int      `json:"total"`
		Processed int      `json:"processed"`
		Skipped   int      `json:"skipped"`
		Failed    int      `json:"failed"`
		Planned   int      `json:"planned,omitempty"`
		DryRun    bool     `json:"dry_run,omitempty"`
		Duration  string   `json:"duration"`
		Results   []Result `json:"results"`
	}
	res := r.Results
	if res == nil {
		res = []Result{}
	}
	return json.Marshal(out{
		Total: r.Total, Processed: r.Processed, Skipped: r.Skipped,
		Failed: r.Failed, Planned: r.Planned, DryRun: r.DryRun,
		Duration: r.Duration.Round(100 * time.Millisecond).String(),
		Results:  res,
	})
}
