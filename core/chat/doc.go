// Package chat defines provider-neutral chat protocol values and minimal
// calling capabilities.
//
// Protocol values in this package are serializable and do not retain tool
// executors, registries, middleware state, provider clients, or other runtime
// objects. Higher-level orchestration belongs in packages outside Core.
package chat
