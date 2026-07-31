// Package prodia wraps Prodia's image generation REST API.
//
// [NewImageModel] targets the synchronous POST /v2/job endpoint for
// text-to-image job types. [image.Options].Model is the official job type
// discriminator, such as "inference.flux-fast.schnell.txt2img.v2"; the
// type-specific config is available through [JobRequest] under [ImageRequestExtensionKey].
// PNG, JPEG, and WebP output use the endpoint's standard Accept negotiation.
//
// See https://docs.prodia.com/reference/inference/ for the full reference.
package prodia
