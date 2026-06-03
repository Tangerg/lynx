// Package midjourney wraps third-party Midjourney proxy APIs (Midjourney
// does not publish a first-party REST endpoint as of 2025). Several
// services — APIFrame, ImaginePro, UseAPI, GoAPI / MJ-API, TTAPI —
// expose a similar shape: POST /imagine returns a task id, GET /fetch/{id}
// (or /task/{id}) returns the image URL once the task completes.
//
// BaseURL is therefore a REQUIRED config knob: the caller picks the
// proxy they hold an account with. The package follows the most
// commonly-published shape (APIFrame / ImaginePro variant). When a
// specific proxy diverges, callers can thread overrides via
// [GenerateRequest] Extra.
package midjourney
