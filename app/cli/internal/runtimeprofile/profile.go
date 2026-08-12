// Package runtimeprofile defines the immutable runtime-discovery projection
// consumed by CLI capability gates, diagnostics, and delivery adapters.
package runtimeprofile

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

type Protocol struct {
	Current      string `json:"current"`
	MinSupported string `json:"minSupported"`
}

type Server struct {
	Name             string `json:"name"`
	Version          string `json:"version"`
	DefaultWorkspace string `json:"defaultWorkspace"`
	Home             string `json:"home"`
}

type Stability string

const (
	Stable       Stability = "stable"
	Experimental Stability = "experimental"
)

type Feature struct {
	Enabled               bool      `json:"enabled"`
	Stability             Stability `json:"stability"`
	ClientOptIn           bool      `json:"clientOptIn"`
	ClientRequested       bool      `json:"clientRequested"`
	RequiredByRunProtocol bool      `json:"requiredByRunProtocol"`
}

// Available reports whether the server and this client agreed to use the
// feature. Server support alone is insufficient for opt-in capabilities.
func (feature Feature) Available() bool {
	return feature.Enabled && (!feature.ClientOptIn || feature.ClientRequested)
}

type Snapshot struct {
	Key            string `json:"key"`
	RecoveryMethod string `json:"recoveryMethod"`
	Scope          string `json:"scope"`
	Writer         string `json:"writer"`
}

type ReplayLimits struct {
	Scope     string `json:"scope"`
	MaxEvents int    `json:"maxEvents"`
	MaxBytes  int    `json:"maxBytes"`
}

type SubscriptionLimits struct {
	MaxTopics  int `json:"maxTopics"`
	MaxWatches int `json:"maxWatches"`
}

type Limits struct {
	MaxConcurrentRuns                int                `json:"maxConcurrentRuns"`
	IdempotencyRetentionSeconds      int                `json:"idempotencyRetentionSeconds"`
	RunReplay                        ReplayLimits       `json:"runReplay"`
	MCPAuthorizationRetentionSeconds int                `json:"mcpAuthorizationRetentionSeconds"`
	RuntimeSubscription              SubscriptionLimits `json:"runtimeSubscription"`
}

// Profile is the complete, CLI-owned projection of one successful runtime
// discovery. It is immutable by convention; Clone crosses ownership boundaries.
type Profile struct {
	Protocol         Protocol           `json:"protocol"`
	Server           Server             `json:"server"`
	RunEvents        []string           `json:"runEvents"`
	RuntimeTopics    []string           `json:"runtimeTopics"`
	StateSnapshots   []Snapshot         `json:"stateSnapshots"`
	StreamingMethods []string           `json:"streamingMethods"`
	Features         map[string]Feature `json:"features"`
	Limits           Limits             `json:"limits"`
}

func (profile Profile) Clone() Profile {
	profile.RunEvents = slices.Clone(profile.RunEvents)
	profile.RuntimeTopics = slices.Clone(profile.RuntimeTopics)
	profile.StateSnapshots = slices.Clone(profile.StateSnapshots)
	profile.StreamingMethods = slices.Clone(profile.StreamingMethods)
	profile.Features = maps.Clone(profile.Features)
	return profile
}

// Available reports whether discovery populated this profile.
func (profile Profile) Available() bool {
	return strings.TrimSpace(profile.Server.Name) != ""
}

func (profile Profile) Validate() error {
	var problems []error
	if strings.TrimSpace(profile.Protocol.Current) == "" || strings.TrimSpace(profile.Protocol.MinSupported) == "" {
		problems = append(problems, errors.New("protocol range is incomplete"))
	}
	if strings.TrimSpace(profile.Server.Name) == "" || strings.TrimSpace(profile.Server.Version) == "" {
		problems = append(problems, errors.New("server identity is incomplete"))
	}
	if strings.TrimSpace(profile.Server.DefaultWorkspace) == "" || strings.TrimSpace(profile.Server.Home) == "" {
		problems = append(problems, errors.New("server filesystem context is incomplete"))
	}
	for name, values := range map[string][]string{
		"run events": profile.RunEvents, "runtime topics": profile.RuntimeTopics,
		"streaming methods": profile.StreamingMethods,
	} {
		if err := validateUniqueStrings(name, values); err != nil {
			problems = append(problems, err)
		}
	}
	seenSnapshots := make(map[string]struct{}, len(profile.StateSnapshots))
	for index, snapshot := range profile.StateSnapshots {
		if strings.TrimSpace(snapshot.Key) == "" || strings.TrimSpace(snapshot.RecoveryMethod) == "" ||
			strings.TrimSpace(snapshot.Scope) == "" || strings.TrimSpace(snapshot.Writer) == "" {
			problems = append(problems, fmt.Errorf("state snapshot %d is incomplete", index+1))
		}
		if _, duplicate := seenSnapshots[snapshot.Key]; duplicate {
			problems = append(problems, fmt.Errorf("state snapshots repeat %q", snapshot.Key))
		}
		seenSnapshots[snapshot.Key] = struct{}{}
	}
	for name, feature := range profile.Features {
		if strings.TrimSpace(name) == "" {
			problems = append(problems, errors.New("feature name is empty"))
		}
		if feature.Stability != Stable && feature.Stability != Experimental {
			problems = append(problems, fmt.Errorf("feature %q has invalid stability %q", name, feature.Stability))
		}
	}
	if err := profile.Limits.validate(); err != nil {
		problems = append(problems, err)
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("runtime profile: %w", err)
	}
	return nil
}

func (limits Limits) validate() error {
	if limits.MaxConcurrentRuns < 0 {
		return errors.New("runtime limits have a negative concurrent-run cap")
	}
	if limits.IdempotencyRetentionSeconds <= 0 || limits.MCPAuthorizationRetentionSeconds <= 0 {
		return errors.New("runtime limits require positive retention periods")
	}
	if strings.TrimSpace(limits.RunReplay.Scope) == "" || limits.RunReplay.MaxEvents <= 0 || limits.RunReplay.MaxBytes <= 0 {
		return errors.New("runtime replay limits are incomplete")
	}
	if limits.RuntimeSubscription.MaxTopics <= 0 || limits.RuntimeSubscription.MaxWatches <= 0 {
		return errors.New("runtime subscription limits must be positive")
	}
	return nil
}

func validateUniqueStrings(name string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s item %d is empty", name, index+1)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("%s repeat %q", name, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func (profile Profile) Supports(feature string) bool {
	return profile.Features[feature].Available()
}

func (profile Profile) SupportsRuntimeTopic(topic string) bool {
	return slices.Contains(profile.RuntimeTopics, topic)
}

func (profile Profile) AvailableFeatureNames() []string {
	names := make([]string, 0, len(profile.Features))
	for name, feature := range profile.Features {
		if feature.Available() {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}
