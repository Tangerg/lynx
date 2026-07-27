// Package event defines the framework's typed lifecycle events and concurrent
// multicast listener. Runtime publishers and subscribers exchange Event values
// directly; listeners may use a type switch when they need an event-specific
// payload.
package event
