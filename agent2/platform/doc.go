// Package platform provides optional multi-Deployment catalog, routing, and
// governance capabilities above the Agent Engine.
//
// Catalog snapshots contain exact immutable Deployment bindings and implement
// agent2.DeploymentResolver without owning Process lifecycle. Mutable deployment
// commands, routing policy, and observation build on snapshots in this package;
// they do not move application persistence or product policy into the Framework.
package platform
