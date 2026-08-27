package embedded

import (
	"github.com/Tangerg/scope/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

// CallOptions describes one read or other non-mutating operation.
type CallOptions struct {
	RequestMeta protocol.RequestMeta
}

// CommandOptions describes one mutation and its optional stable retry identity.
type CommandOptions struct {
	RequestMeta          protocol.RequestMeta
	IdempotencyKey       string
	IdempotencyNamespace string
}

// RunCommandOptions describes a Run-opening command. AfterEventID applies only
// when an idempotent retry reattaches to the Run opened by the first attempt.
type RunCommandOptions struct {
	RequestMeta          protocol.RequestMeta
	IdempotencyKey       string
	IdempotencyNamespace string
	AfterEventID         string
}

// RunSubscriptionOptions identifies the last Run event already consumed.
type RunSubscriptionOptions struct {
	RequestMeta  protocol.RequestMeta
	AfterEventID string
}

// SubscriptionOptions describes a non-replayable Runtime-wide subscription.
type SubscriptionOptions struct {
	RequestMeta protocol.RequestMeta
}

func callOptions(options CallOptions) operation.Options {
	return operation.Options{RequestMeta: currentRequestMeta(options.RequestMeta)}
}

func commandOptions(options CommandOptions) operation.Options {
	return operation.Options{
		RequestMeta:          currentRequestMeta(options.RequestMeta),
		IdempotencyKey:       options.IdempotencyKey,
		IdempotencyNamespace: options.IdempotencyNamespace,
	}
}

func runCommandOptions(options RunCommandOptions) operation.Options {
	return operation.Options{
		RequestMeta:          currentRequestMeta(options.RequestMeta),
		IdempotencyKey:       options.IdempotencyKey,
		IdempotencyNamespace: options.IdempotencyNamespace,
		AfterEventID:         options.AfterEventID,
	}
}

func runSubscriptionOptions(options RunSubscriptionOptions) operation.Options {
	return operation.Options{
		RequestMeta:  currentRequestMeta(options.RequestMeta),
		AfterEventID: options.AfterEventID,
	}
}

func subscriptionOptions(options SubscriptionOptions) operation.Options {
	return operation.Options{RequestMeta: currentRequestMeta(options.RequestMeta)}
}

func currentRequestMeta(meta protocol.RequestMeta) protocol.RequestMeta {
	if meta.ProtocolVersion == "" {
		meta.ProtocolVersion = protocol.ProtocolVersion
	}
	return meta
}
