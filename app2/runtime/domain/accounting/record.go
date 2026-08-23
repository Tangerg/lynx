// Package accounting owns the durable attribution facts used by usage reports.
package accounting

import "time"

type RunRecord struct {
	SessionID string
	Provider string
	Model string
	Body []byte
	FinishedAt time.Time
}
