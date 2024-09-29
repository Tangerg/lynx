package model

import (
	"context"
)

// Flux is a generic interface that defines methods for writing, processing, and reading data.
// T is a type parameter that can be any type, allowing for flexibility in the data types used.
type Flux[T any] interface {
	// Write takes a context and a byte slice as input and returns an error.
	// It is responsible for writing the provided data.
	Write(ctx context.Context, data []byte) error

	// Process takes a context and a byte slice as input and returns a value of type T and an error.
	// It processes the provided data and returns the result of type T.
	Process(ctx context.Context, data []byte) (T, error)

	// Read takes a context and a value of type T as input and returns an error.
	// It reads or retrieves data based on the provided value of type T.
	Read(ctx context.Context, t T) error
}
