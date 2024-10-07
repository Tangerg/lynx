package function

// Options is an interface that defines the contract for managing configuration
// options related to functions and proxy usage within a system. It provides methods
// to retrieve and set functions, as well as to enable or disable proxy usage.
//
// Methods:
//
// Functions() []Function
//   - Returns a slice of Function instances.
//   - This method provides access to the current set of functions configured in the system,
//     allowing for retrieval and iteration over available functions.
//
// SetFunctions(funcs []Function)
//   - Sets the functions to be used in the system.
//   - `funcs` is a slice of Function instances that will replace the current set of functions.
//   - This method allows for updating or configuring the functions available for execution.
//
// ProxyToolCalls() bool
//   - Returns a boolean indicating whether proxy usage is enabled.
//   - This method provides access to the current proxy configuration, which determines
//     whether certain internal functions should be bypassed and handled externally.
//
// SetProxyToolCalls(enable bool)
//   - Enables or disables proxy usage based on the provided boolean value.
//   - `enable` is a boolean that determines whether the system should bypass certain
//     internal functions and delegate their handling to external processes.
//   - This method allows for configuring the system to either handle functions internally
//     or to offload them to external handlers, depending on the specified setting.
type Options interface {
	Functions() []Function
	SetFunctions(funcs []Function)
	ProxyToolCalls() bool
	SetProxyToolCalls(enable bool)
}
