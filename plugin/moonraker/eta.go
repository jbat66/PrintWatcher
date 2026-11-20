package moonraker

import (
	"context"
	"net/url"
	"time"
)

type completedPrint struct {
	Filename string
	Status   string
	Exists   bool
	Duration float64 `json:"print_duration"`
}

// Moonraker marks exists false if the G-code has changed since a historical
// print. Only reuse successful runs of the unchanged file, and use printing
// duration so pauses and preparation do not inflate the baseline.
func (p *Plugin) completedDuration(ctx context.Context, filename string) float64 {
	p.etaMu.Lock()
	defer p.etaMu.Unlock()
	if time.Since(p.etaHistoryAt) >= time.Minute {
		p.etaHistoryAt = time.Now()
		p.etaHistory = nil
		var result struct{ Jobs []completedPrint }
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if p.query(ctx, "server/history/list", url.Values{"limit": {"100"}, "order": {"desc"}}, &result) == nil {
			p.etaHistory = result.Jobs
		}
	}
	for _, job := range p.etaHistory {
		if job.Filename == filename && job.Exists && job.Status == "completed" && job.Duration > 0 {
			return job.Duration
		}
	}
	return 0
}
