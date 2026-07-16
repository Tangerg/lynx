// Package prodia wraps Prodia's image generation REST API.
//
// [NewImageModel] targets Prodia's /v2/job/start + /v2/job/{id}
// endpoints, supporting a catalog of Stable-Diffusion / SDXL / SD3 /
// FLUX checkpoints and LoRAs. Model selection is via the upstream
// "model" field (extension-threaded) — Prodia model ids look like
// "sd_xl_base_1.0.safetensors [be9edd61]" rather than the canonical
// brand names.
//
// See https://docs.prodia.com/ for the full reference.
package prodia
