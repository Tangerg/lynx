// Package platform provides optional multi-Deployment catalog, routing, and
// governance capabilities above the Agent Engine.
//
// Catalog snapshots contain exact immutable Deployment bindings and implement
// agent.DeploymentResolver without owning Process lifecycle. Mutable deployment
// commands and routing policy build on these snapshots; application persistence,
// product policy, and observation backends remain outside this package.
package platform
