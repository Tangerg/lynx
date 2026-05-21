// Package httpreq exposes a single LLM-callable HTTP-request tool. It
// wraps go-resty as the transport and enforces a host allowlist +
// method allowlist + response-size cap to keep the LLM from reaching
// internal services or blowing up the context window.
//
// The allowlist is mandatory — there is no "allow all" mode. Callers
// MUST enumerate the hosts the LLM is permitted to reach.
package httpreq
