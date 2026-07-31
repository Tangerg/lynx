// Package vertexai exposes Google Gen AI capabilities through Vertex AI's GCP
// backend and Application Default Credentials.
//
// Chat, transcription, speech, and text embedding share the official Google
// Gen AI SDK protocol mappers with package google while supplying
// genai.BackendVertexAI, project, and location. Image generation has its own
// adapter because the current Vertex contract is GenerateContent with Gemini
// image models, whereas the public Gemini Developer API now recommends the
// Interactions API. Treating those two transports as interchangeable would
// leak backend details and preserve deprecated Imagen behavior.
//
// API keys are not used. Authenticate locally with Application Default
// Credentials or provide an authenticated HTTP client; production workloads
// normally use a service account or Workload Identity.
//
// Model availability and regions change independently on Vertex AI. Select an
// explicit model id and location from the current Vertex model documentation.
//
// See https://cloud.google.com/vertex-ai/generative-ai/docs.
package vertexai
