package model

import "context"

type StreamingModel[Treq any, OReq Options, TRes any, MRes ResultMetadata] interface {
	Stream(ctx context.Context, req Request[Treq, OReq], flux Flux[Response[TRes, MRes]]) error
}
