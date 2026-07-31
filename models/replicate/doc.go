// Package replicate implements Replicate's official prediction-job protocol
// and schema-bound image and speech adapters.
//
// Replicate is not a modality API with one shared image or TTS request shape.
// Every model version publishes an independent OpenAPI input/output schema.
// Accordingly, ImageModel and AudioTTSModel require an explicit schema binding
// at construction and reject model overrides. Provider-specific fields remain
// in PredictionRequest.Input under ImageRequestExtensionKey or
// SpeechRequestExtensionKey; Core fields are mapped only through the declared
// binding. This prevents a field named "seed", "voice", or "width" on one
// model from being guessed for an unrelated model.
//
// Model identifiers use two official forms:
//
//   - owner/name for official models, sent to
//     /v1/models/{owner}/{name}/predictions;
//   - owner/name:version for community models, sent to /v1/predictions with
//     the immutable version in the request body.
//
// Predictions run asynchronously. The high-level adapters submit, poll to a
// terminal state, validate the configured output schema, and copy ephemeral
// output files before Replicate removes API prediction data.
//
// See https://replicate.com/docs/reference/http and each model's API schema.
package replicate
