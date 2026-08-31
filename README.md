# Scope

Scope is a modular Go foundation for building AI systems. It provides portable contracts and composable infrastructure for models, agents, data pipelines, retrieval, evaluation, tools, interoperability, and observability.

## Position

Scope is a framework and library. Applications select the modules they need and compose them through explicit Go contracts. Provider and storage integrations remain optional, independently versioned leaves.

## Boundaries

Scope owns reusable AI infrastructure semantics. It does not own product sessions, dashboards, user identity, billing, marketplaces, deployment catalogs, application persistence, or desktop workflows. Those belong to the host application, such as Flame.

Each capability has one owning package and one public representation. Scope avoids root façades, hidden registration, ambient authority, and provider details in shared contracts.

## Evolution

Modules follow semantic versioning independently. Before version 1, a minor release may replace a wrong public design without retaining aliases or compatibility shims. Consumers should pin the module versions they adopt and review minor upgrades.

## License

Scope is licensed under the [Apache License 2.0](LICENSE).
