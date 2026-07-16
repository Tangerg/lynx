// Package stability wraps Stability AI's image generation REST API.
//
// [NewImageModel] targets the v2beta /stable-image/generate endpoints
// (ultra, core, sd3, etc.) — the v1 /v1/generation surface is legacy
// and not exposed here. Per-model knobs (aspect_ratio, negative_prompt,
// seed, style_preset, output_format) ride through the typed
// [image.Options] fields plus extension-threaded params.
//
// Stability's edit / upscale / control / video surfaces ship at sibling
// paths under /v2beta but require a different request shape; they're
// not modeled here.
//
// See https://platform.stability.ai/docs/api-reference for the full
// reference.
package stability
