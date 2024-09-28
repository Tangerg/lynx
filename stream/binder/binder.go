package binder

import (
	"github.com/Tangerg/lynx/stream/binding"
)

// Binder
// create binding of produce or consume types
type Binder interface {

	// BindProducer
	//create a produce type binding, can only send messages and confirm messages
	BindProducer(destination string) (binding.Binding, error)

	// BindConsumer
	//create a consume type binding, can only receive messages and confirm messages
	BindConsumer(destination string) (binding.Binding, error)
}
