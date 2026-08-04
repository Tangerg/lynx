// Package domain contains the product's bounded contexts: one sub-package per
// business capability, holding entities, value objects, domain services, and
// consumer-owned ports. Domain packages are independent of transport, storage,
// and integration implementations.
//
// A bounded context that needs replaceable storage or policy evaluation
// (session, knowledge, transcript, provider, interrupts, approval, …) defines a
// consumer-side Store, Registry, or Policy interface named for the capability
// it provides. Implementations live outside the domain layer.
package domain
