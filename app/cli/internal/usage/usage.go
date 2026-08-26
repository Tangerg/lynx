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

func (t Totals) Validate() error {
	if t.InputTokens < 0 || t.OutputTokens < 0 || t.CacheReadTokens < 0 ||
		t.CacheWriteTokens < 0 || t.ReasoningTokens < 0 {
		return errors.New("usage totals contain a negative token count")
	}
	if t.CostUSD != nil && *t.CostUSD < 0 {
		return errors.New("usage totals contain a negative cost")
	}
	return nil
}

type Bucket struct {
	Key    string
	Totals Totals
	Runs   int
}

func (b Bucket) Validate() error {
	if strings.TrimSpace(b.Key) == "" {
		return errors.New("usage bucket key is empty")
	}
	if b.Runs < 0 {
		return errors.New("usage bucket run count is negative")
	}
	return b.Totals.Validate()
}

type SessionReport struct {
	SessionID string
	Total     Totals
	ByModel   []Bucket
}

func (s SessionReport) Validate() error {
	if strings.TrimSpace(s.SessionID) == "" {
		return errors.New("session usage report id is empty")
	}
	if err := s.Total.Validate(); err != nil {
		return fmt.Errorf("session usage report: %w", err)
	}
	return validateBuckets("session usage report", s.ByModel)
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

func (s Summary) Validate() error {
	if s.SinceDays < 0 || s.Sessions < 0 || s.Runs < 0 {
		return errors.New("usage summary contains a negative count")
	}
	if err := s.Total.Validate(); err != nil {
		return fmt.Errorf("usage summary: %w", err)
	}
	for _, breakdown := range []struct {
		name    string
		buckets []Bucket
	}{
		{name: "provider", buckets: s.ByProvider},
		{name: "model", buckets: s.ByModel},
		{name: "day", buckets: s.ByDay},
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
