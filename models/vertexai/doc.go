// Package vertexai wraps Google Cloud's Vertex AI generative surface.
//
// Vertex AI hosts the same Gemini / Imagen / embedding models as the
// public Gemini API but routes through GCP infrastructure: auth flows
// through Application Default Credentials (gcloud auth login, service
// accounts, Workload Identity), models are addressed under a GCP
// project + region, and quotas come from the GCP billing account
// rather than an api-key allowance.
//
// This package is a thin facade over [models/google]: every constructor
// pre-fills [genai.BackendVertexAI] and forwards the typed Project /
// Location knobs to genai.ClientConfig. The returned model types are
// the same [google.ChatModel] / [google.EmbeddingModel] / etc — so
// downstream code works identically across the two backends.
//
// Auth note: ApiKey is not used here. Vertex authenticates via ADC
// (https://cloud.google.com/docs/authentication/application-default-credentials).
// Run "gcloud auth application-default login" locally, or attach a
// service account in production.
//
// Available models on Vertex AI (use the same model id strings you
// would for the Gemini API):
//
//   - gemini-2.5-pro / gemini-2.5-flash / gemini-2.5-flash-lite
//   - gemini-2.0-flash / gemini-2.0-flash-lite
//   - text-embedding-005 / text-multilingual-embedding-002 /
//     gemini-embedding-001
//   - imagen-4.0-generate-001 / imagen-3.0-generate-002
//
// See https://cloud.google.com/vertex-ai/generative-ai/docs for the
// full reference.
package vertexai
