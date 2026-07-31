// Package blackforestlabs wraps Black Forest Labs' FLUX image generation API.
//
// [NewImageModel] targets the official asynchronous /v1/{model} endpoints,
// follows each response's polling_url, and downloads the short-lived signed
// output before returning it.
//
// FLUX-specific knobs (steps, guidance, raw, safety_tolerance,
// output_format, prompt_upsampling, image_prompt for img2img /
// kontext editing) ride through extension-threaded params.
//
// See https://docs.bfl.ai/ for the full reference.
package blackforestlabs
