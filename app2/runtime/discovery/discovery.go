package discovery

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type Config struct {
	ServerInfo           protocol.ServerInfo
	IdempotencyNamespace string
	EnabledFeatures      map[string]bool
	RunEvents            []protocol.StreamEventType
	RuntimeTopics        []protocol.RuntimeTopic
	StreamingMethods     []string
}

type Service struct {
	response protocol.DiscoverResponse
}

func New(config Config) (*Service, error) {
	if config.IdempotencyNamespace == "" {
		return nil, errors.New("discovery: idempotency namespace is required")
	}
	capabilities, err := capabilities(config)
	if err != nil {
		return nil, err
	}
	response := protocol.DiscoverResponse{
		ProtocolVersion: protocol.ProtocolVersion,
		ServerInfo:      config.ServerInfo,
		Capabilities:    capabilities,
	}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("discovery: invalid response: %w", err)
	}
	return &Service{response: response}, nil
}

func (service *Service) Discover(ctx context.Context) (*protocol.DiscoverResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	response := service.response
	response.Capabilities.RunEvents = cloneList(response.Capabilities.RunEvents)
	response.Capabilities.RuntimeTopics = cloneList(response.Capabilities.RuntimeTopics)
	response.Capabilities.StreamingMethods = cloneList(response.Capabilities.StreamingMethods)
	response.Capabilities.Features = maps.Clone(response.Capabilities.Features)
	return &response, nil
}

func capabilities(config Config) (protocol.ServerCapabilities, error) {
	features := make(map[string]protocol.FeatureCapability, len(protocol.Features()))
	for _, feature := range protocol.Features() {
		features[feature.Key] = protocol.FeatureCapability{
			Enabled:               config.EnabledFeatures[feature.Key],
			ClientOptIn:           feature.ClientOptIn,
			RequiredByRunProtocol: feature.RequiredByRunProtocol,
		}
	}
	for key := range config.EnabledFeatures {
		if _, published := protocol.LookupFeature(key); !published {
			return protocol.ServerCapabilities{}, fmt.Errorf("discovery: unpublished feature %q", key)
		}
	}
	return protocol.ServerCapabilities{
		RunEvents:        cloneList(config.RunEvents),
		RuntimeTopics:    cloneList(config.RuntimeTopics),
		StreamingMethods: cloneList(config.StreamingMethods),
		Features:         features,
		Limits: protocol.RuntimeLimits{
			Idempotency: protocol.IdempotencyLimits{
				RetentionSeconds: protocol.DefaultIdempotencyTTL,
				Namespace:        config.IdempotencyNamespace,
			},
			RunReplay: protocol.RunReplayLimits{
				Scope:     protocol.ReplayScopeRuntimeInstanceRootSegment,
				MaxEvents: protocol.DefaultReplayEvents,
				MaxBytes:  protocol.DefaultReplayBytes,
			},
			MCPAuthorizationAttempts: protocol.MCPAuthorizationAttemptLimits{
				RetentionSeconds: protocol.DefaultMCPAttemptTTL,
			},
			RuntimeSubscription: protocol.SubscriptionLimits{
				MaxTopics:  protocol.MaxSubscriptionTopics,
				MaxWatches: protocol.MaxSubscriptionWatches,
			},
		},
	}, nil
}

func cloneList[T any](values []T) []T {
	if len(values) == 0 {
		return []T{}
	}
	return slices.Clone(values)
}
