// Package domain is the bounded-context layer of the Clean Arch ring: one
// sub-package per business capability, each holding entities, domain services,
// and consumer-side ports. The capabilities are composed by the application and
// adapter layers and exposed at the wire by delivery; the domain packages
// themselves know nothing of transports, wire formats, or driven adapters.
//
// A bounded context that needs replaceable storage or policy evaluation
// (session, knowledge, transcript, provider, interrupts, approval, …) defines a
// consumer-side Store / Registry / Policy interface and an implementation named
// for its essence (sqlite-backed, file-backed, adapter-backed). See
// lyra/doc/EXECUTION_CENTERED_ARCHITECTURE.md.
package domain
