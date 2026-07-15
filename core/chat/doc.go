// Package chat defines the serializable provider-neutral chat protocol and its
// minimal synchronous [Model] and optional [Streamer] capabilities.
//
// Construct messages and requests with NewSystemMessage, NewUserMessage, and
// NewRequest. Constructors validate their initial values; call Validate again
// after mutating exported fields. Options express only per-call overrides, and
// namespaced Extensions preserve provider data without expanding the shared
// protocol for every provider feature.
//
// ToolDefinition describes wire schema only. Executable tools, registries,
// history, retries, middleware policy, and tool loops belong to higher-level
// modules. Protocol values therefore never retain callbacks, provider clients,
// or other runtime objects.
package chat
