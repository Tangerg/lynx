// Package blackforestlabs wraps Black Forest Labs' FLUX image generation API.
//
// [NewImageModel] targets the async /v1/{model} endpoints (flux-pro,
// flux-pro-1.1, flux-dev, flux-schnell, flux-kontext-pro for in-
// painting/edits). The package handles the submit → poll → fetch
// cycle and returns the generated image URL(s).
//
// FLUX-specific knobs (steps, guidance, raw, safety_tolerance,
// output_format, prompt_upsampling, image_prompt for img2img /
// kontext editing) ride through extension-threaded params.
//
// See https://docs.bfl.ml/ for the full reference.
package blackforestlabs
