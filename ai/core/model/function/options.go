package function

// Options defines the contract for managing configuration options related
// to functions and proxy usage within a system. This interface facilitates
// retrieval and modification of function configurations, as well as enabling
// or disabling proxy behavior for handling certain functions.
//
// Methods:
//
// Functions:
//
//	Functions() []Function
//	Retrieves the current set of functions configured in the system.
//	Returns:
//	- []Function: A slice of Function instances representing the available functions.
//
// SetFunctions:
//
//	SetFunctions(funcs []Function)
//	Updates the set of functions to be used in the system.
//	Parameters:
//	- funcs: A slice of Function instances that will replace the current set of functions.
//
// ProxyToolCalls:
//
//	ProxyToolCalls() bool
//	Indicates whether proxy usage is currently enabled. Proxy usage determines
//	whether certain internal functions are bypassed and handled externally.
//	Returns:
//	- bool: True if proxy usage is enabled, false otherwise.
//
// SetProxyToolCalls:
//
//	SetProxyToolCalls(enable bool)
//	Enables or disables proxy usage based on the provided boolean value.
//	Parameters:
//	- enable: A boolean indicating whether to enable (true) or disable (false)
//	  proxy usage. When enabled, certain internal functions may be delegated
//	  to external handlers.
//
// Example Implementation:
//
//	type MyOptions struct {
//	    funcs      []Function
//	    proxyCalls bool
//	}
//
//	func (o *MyOptions) Functions() []Function {
//	    return o.funcs
//	}
//
//	func (o *MyOptions) SetFunctions(funcs []Function) {
//	    o.funcs = funcs
//	}
//
//	func (o *MyOptions) ProxyToolCalls() bool {
//	    return o.proxyCalls
//	}
//
//	func (o *MyOptions) SetProxyToolCalls(enable bool) {
//	    o.proxyCalls = enable
//	}
type Options interface {
	// Functions retrieves the current set of functions configured in the system.
	Functions() []Function

	// SetFunctions updates the set of functions to be used in the system.
	SetFunctions(funcs []Function)

	// ProxyToolCalls indicates whether proxy usage is currently enabled.
	ProxyToolCalls() bool

	// SetProxyToolCalls enables or disables proxy usage based on the provided boolean value.
	SetProxyToolCalls(enable bool)
}
