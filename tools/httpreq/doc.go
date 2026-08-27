// Package httpreq exposes a single model-callable HTTP-request tool. It wraps
// go-resty as the transport and enforces host, method, redirect, timeout, and
// response-size policy at one client boundary.
//
// The allowlist is mandatory — there is no "allow all" mode. Callers
// MUST enumerate the hosts the LLM is permitted to reach.
package httpreq
