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
	Version string `json:"version"`
}

type Server struct {
	Name             string `json:"name"`
	Version          string `json:"version"`
	DefaultWorkspace string `json:"defaultWorkspace"`
	Home             string `json:"home"`
}

// FeatureName is the runtime capability vocabulary the CLI currently knows how
// to consume. Discovery may still carry newer names; FeatureName remains a
// string so the profile can preserve and report them without interpreting them.
type FeatureName string

const (
	FeatureReasoning     FeatureName = "reasoning"
	FeatureMultimodal    FeatureName = "multimodal"
	FeatureCompaction    FeatureName = "compaction"
	FeaturePlan          FeatureName = "plan"
	FeatureGoals         FeatureName = "goals"
	FeatureAgentMemory   FeatureName = "agentMemory"
	FeatureKnowledge     FeatureName = "knowledge"
	FeatureSkills        FeatureName = "skills"
	FeatureMCP           FeatureName = "mcp"
	FeatureSchedules     FeatureName = "schedules"
	FeatureGit           FeatureName = "git"
	FeatureCheckpoints   FeatureName = "checkpoints"
	FeatureFileWatch     FeatureName = "fileWatch"
	FeatureLSP           FeatureName = "lsp"
	FeatureSessionExport FeatureName = "sessionExport"
	FeatureRelocate      FeatureName = "relocate"
	FeatureSubagents     FeatureName = "subagents"
)

// KnownFeatures returns the runtime feature vocabulary understood by this CLI.
// Profiles still preserve unknown future names for diagnostics.
func KnownFeatures() []FeatureName {
	return []FeatureName{
		FeatureReasoning, FeatureMultimodal, FeatureCompaction, FeaturePlan,
		FeatureGoals, FeatureAgentMemory, FeatureKnowledge, FeatureSkills,
		FeatureMCP, FeatureSchedules, FeatureGit,
		FeatureCheckpoints, FeatureFileWatch, FeatureLSP, FeatureSessionExport,
		FeatureRelocate, FeatureSubagents,
	}
}

type Feature struct {
	Enabled               bool `json:"enabled"`
	ClientOptIn           bool `json:"clientOptIn"`
	ClientRequested       bool `json:"clientRequested"`
	RequiredByRunProtocol bool `json:"requiredByRunProtocol"`
}

// Available reports whether the server and this client agreed to use the
// feature. Server support alone is insufficient for opt-in capabilities.
func (feature Feature) Available() bool {
	return feature.Enabled && (!feature.ClientOptIn || feature.ClientRequested)
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
	IdempotencyNamespace             string             `json:"idempotencyNamespace"`
	RunReplay                        ReplayLimits       `json:"runReplay"`
	MCPAuthorizationRetentionSeconds int                `json:"mcpAuthorizationRetentionSeconds"`
	RuntimeSubscription              SubscriptionLimits `json:"runtimeSubscription"`
}

// Profile is the complete, CLI-owned projection of one successful runtime
// discovery. It is immutable by convention; Clone crosses ownership boundaries.
type Profile struct {
	Protocol         Protocol                `json:"protocol"`
	Server           Server                  `json:"server"`
	RunEvents        []string                `json:"runEvents"`
	RuntimeTopics    []string                `json:"runtimeTopics"`
	StreamingMethods []string                `json:"streamingMethods"`
	Features         map[FeatureName]Feature `json:"features"`
	Limits           Limits                  `json:"limits"`
}

func (profile Profile) Clone() Profile {
	profile.RunEvents = slices.Clone(profile.RunEvents)
	profile.RuntimeTopics = slices.Clone(profile.RuntimeTopics)
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
	if strings.TrimSpace(profile.Protocol.Version) == "" {
		problems = append(problems, errors.New("protocol version is empty"))
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
	for name := range profile.Features {
		if strings.TrimSpace(string(name)) == "" {
			problems = append(problems, errors.New("feature name is empty"))
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
	if strings.TrimSpace(limits.IdempotencyNamespace) == "" {
		return errors.New("runtime limits require an idempotency namespace")
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

func (profile Profile) Supports(feature FeatureName) bool {
	return profile.Features[feature].Available()
}

func (profile Profile) SupportsRuntimeTopic(topic string) bool {
	return slices.Contains(profile.RuntimeTopics, topic)
}

func (profile Profile) AvailableFeatureNames() []string {
	names := make([]string, 0, len(profile.Features))
	for name, feature := range profile.Features {
		if feature.Available() {
			names = append(names, string(name))
		}
	}
	slices.Sort(names)
	return names
}
