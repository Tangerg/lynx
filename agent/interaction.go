package agent

import "github.com/Tangerg/lynx/agent/interaction"

// Framework interaction aliases keep the standard host path discoverable from
// the root package. Advanced consumers may import agent/interaction directly.
type (
	Suspension            = interaction.Suspension
	SuspensionKind        = interaction.SuspensionKind
	InteractionEvent      = interaction.Event
	InteractionEventKind  = interaction.EventKind
	InteractionLimits     = interaction.Limits
	InteractionStopReason = interaction.StopReason
)

const (
	SuspensionSchemaVersion = interaction.SuspensionSchemaVersion
	SuspensionHuman         = interaction.SuspensionHuman
	SuspensionTool          = interaction.SuspensionTool

	InteractionEventModelRequest  = interaction.EventModelRequest
	InteractionEventModelResponse = interaction.EventModelResponse
	InteractionEventToolCall      = interaction.EventToolCall
	InteractionEventToolResult    = interaction.EventToolResult
	InteractionEventPause         = interaction.EventPause
	InteractionEventResume        = interaction.EventResume

	InteractionStopNone   = interaction.StopNone
	InteractionStopBudget = interaction.StopBudget
	InteractionStopSteps  = interaction.StopSteps
)
