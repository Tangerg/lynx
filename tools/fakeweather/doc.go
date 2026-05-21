// Package fakeweather is a chat.Tool that returns synthesized
// weather data for a given (location, date). All values are
// deterministic functions of the input — there is no real weather API
// behind it.
//
// Use this for demos, prototypes, integration tests, or any flow that
// needs a "weather tool" that produces sane, reproducible JSON without
// network access. NEVER use the output for real decisions.
//
// Determinism: a request that supplies (location, date, includes)
// produces the same Response across runs. The seed is derived from
// the location string and target date.
package fakeweather
