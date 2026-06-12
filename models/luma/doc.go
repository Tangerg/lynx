// Package luma wraps Luma AI's image generation API.
//
// [NewImageModel] targets Luma's async /dream-machine/v1/generations/
// image endpoint (the Photon family). The model targets photorealism
// and image editing; submit → poll → fetch is orchestrated here.
//
// Note: Luma's flagship surface is video generation (Dream Machine /
// Ray-2). Video isn't modeled by the framework — only Luma's image side fits
// core/model/image's interface.
//
// See https://docs.lumalabs.ai/ for the full reference.
package luma
