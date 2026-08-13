package runtimeembedded

import (
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
)

func projectRuntimeProfile(
	discovery *protocol.DiscoverResponse,
	client *protocol.ClientCapabilities,
) (runtimeprofile.Profile, error) {
	if discovery == nil {
		return runtimeprofile.Profile{}, fmt.Errorf("project runtime profile: discovery is nil")
	}
	profile := runtimeprofile.Profile{
		Protocol: runtimeprofile.Protocol{
			Current: string(discovery.Protocol.Current), MinSupported: string(discovery.Protocol.MinSupported),
		},
		Server: runtimeprofile.Server{
			Name: discovery.ServerInfo.Name, Version: discovery.ServerInfo.Version,
			DefaultWorkspace: discovery.ServerInfo.DefaultWorkspace.Path, Home: discovery.ServerInfo.Home,
		},
		RunEvents:        make([]string, 0, len(discovery.Capabilities.RunEvents)),
		RuntimeTopics:    make([]string, 0, len(discovery.Capabilities.RuntimeTopics)),
		StateSnapshots:   make([]runtimeprofile.Snapshot, 0, len(discovery.Capabilities.StateSnapshots)),
		StreamingMethods: append([]string(nil), discovery.Capabilities.StreamingMethods...),
		Features:         make(map[runtimeprofile.FeatureName]runtimeprofile.Feature, len(discovery.Capabilities.Features)),
		Limits: runtimeprofile.Limits{
			MaxConcurrentRuns:           discovery.Capabilities.Limits.MaxConcurrentRuns,
			IdempotencyRetentionSeconds: discovery.Capabilities.Limits.Idempotency.RetentionSeconds,
			IdempotencyNamespace:        discovery.Capabilities.Limits.Idempotency.Namespace,
			RunReplay: runtimeprofile.ReplayLimits{
				Scope:     string(discovery.Capabilities.Limits.RunReplay.Scope),
				MaxEvents: discovery.Capabilities.Limits.RunReplay.MaxEvents,
				MaxBytes:  discovery.Capabilities.Limits.RunReplay.MaxBytes,
			},
			MCPAuthorizationRetentionSeconds: discovery.Capabilities.Limits.MCPAuthorizationAttempts.RetentionSeconds,
			RuntimeSubscription: runtimeprofile.SubscriptionLimits{
				MaxTopics:  discovery.Capabilities.Limits.RuntimeSubscription.MaxTopics,
				MaxWatches: discovery.Capabilities.Limits.RuntimeSubscription.MaxWatches,
			},
		},
	}
	for _, eventType := range discovery.Capabilities.RunEvents {
		profile.RunEvents = append(profile.RunEvents, string(eventType))
	}
	for _, topic := range discovery.Capabilities.RuntimeTopics {
		profile.RuntimeTopics = append(profile.RuntimeTopics, string(topic))
	}
	for _, snapshot := range discovery.Capabilities.StateSnapshots {
		profile.StateSnapshots = append(profile.StateSnapshots, runtimeprofile.Snapshot{
			Key: string(snapshot.Key), RecoveryMethod: snapshot.RecoveryMethod,
			Scope: string(snapshot.Scope), Writer: string(snapshot.Writer),
		})
	}
	for name, feature := range discovery.Capabilities.Features {
		requested := client != nil && client.Features[name].Enabled
		profile.Features[runtimeprofile.FeatureName(name)] = runtimeprofile.Feature{
			Enabled: feature.Enabled, Stability: runtimeprofile.Stability(feature.Stability),
			ClientOptIn: feature.ClientOptIn, ClientRequested: requested,
			RequiredByRunProtocol: feature.RequiredByRunProtocol,
		}
	}
	if err := profile.Validate(); err != nil {
		return runtimeprofile.Profile{}, fmt.Errorf("project runtime profile: %w", err)
	}
	return profile, nil
}
