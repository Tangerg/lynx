package binder

import (
	"github.com/Tangerg/lynx/stream/binding"
)

// Binder is an interface for creating bindings for message producers and consumers.
type Binder interface {

	// BindProducer creates a binding for a producer type, allowing it to send and confirm messages.
	// It takes a destination string as an argument and returns a binding.Binding and an error.
	BindProducer(destination string) (binding.Binding, error)

	// BindConsumer creates a binding for a consumer type, allowing it to receive and confirm messages.
	// It takes a destination string as an argument and returns a binding.Binding and an error.
	BindConsumer(destination string) (binding.Binding, error)
}
