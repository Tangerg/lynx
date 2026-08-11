// Package usage defines the CLI-owned usage reporting model and read port.
package usage

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Totals struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	CostUSD          *float64
}

func (totals Totals) Validate() error {
	if totals.InputTokens < 0 || totals.OutputTokens < 0 || totals.CacheReadTokens < 0 ||
		totals.CacheWriteTokens < 0 || totals.ReasoningTokens < 0 {
		return errors.New("usage totals contain a negative token count")
	}
	if totals.CostUSD != nil && *totals.CostUSD < 0 {
		return errors.New("usage totals contain a negative cost")
	}
	return nil
}

type Bucket struct {
	Key    string
	Totals Totals
	Runs   int
}

func (bucket Bucket) Validate() error {
	if strings.TrimSpace(bucket.Key) == "" {
		return errors.New("usage bucket key is empty")
	}
	if bucket.Runs < 0 {
		return errors.New("usage bucket run count is negative")
	}
	return bucket.Totals.Validate()
}

type SessionReport struct {
	SessionID string
	Total     Totals
	ByModel   []Bucket
}

func (report SessionReport) Validate() error {
	if strings.TrimSpace(report.SessionID) == "" {
		return errors.New("session usage report id is empty")
	}
	if err := report.Total.Validate(); err != nil {
		return fmt.Errorf("session usage report: %w", err)
	}
	return validateBuckets("session usage report", report.ByModel)
}

type Summary struct {
	SinceDays  int
	Total      Totals
	ByProvider []Bucket
	ByModel    []Bucket
	ByDay      []Bucket
	Sessions   int
	Runs       int
}

func (summary Summary) Validate() error {
	if summary.SinceDays < 0 || summary.Sessions < 0 || summary.Runs < 0 {
		return errors.New("usage summary contains a negative count")
	}
	if err := summary.Total.Validate(); err != nil {
		return fmt.Errorf("usage summary: %w", err)
	}
	for _, breakdown := range []struct {
		name    string
		buckets []Bucket
	}{
		{name: "provider", buckets: summary.ByProvider},
		{name: "model", buckets: summary.ByModel},
		{name: "day", buckets: summary.ByDay},
	} {
		if err := validateBuckets("usage summary "+breakdown.name, breakdown.buckets); err != nil {
			return err
		}
	}
	return nil
}

func validateBuckets(context string, buckets []Bucket) error {
	seen := make(map[string]struct{}, len(buckets))
	for index, bucket := range buckets {
		if err := bucket.Validate(); err != nil {
			return fmt.Errorf("%s bucket %d: %w", context, index+1, err)
		}
		if _, duplicate := seen[bucket.Key]; duplicate {
			return fmt.Errorf("%s repeats bucket %q", context, bucket.Key)
		}
		seen[bucket.Key] = struct{}{}
	}
	return nil
}

type Service interface {
	SessionUsage(context.Context, string) (SessionReport, error)
	Summary(context.Context, int) (Summary, error)
}
