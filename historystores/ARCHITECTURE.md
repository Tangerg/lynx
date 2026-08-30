# historystores architecture

> `core/history` owns the contract, middleware, and in-memory reference
> implementation built on `core/chat.Message`. This namespace holds only the
> external persistence providers.

Repository-wide rules live in [`../AGENTS.md`](../AGENTS.md). The backend
inventory and dependency versions follow the code; the usage entry point is
[`README.md`](README.md).

---

## 1. Position

- **One `core/history.Store` capability model, several composable
  implementations.** A consumer depends only on the small Reader, Writer, or
  Clearer interface it needs.
- Each external provider is an independent leaf module at
  `historystores/<provider>`. There is no aggregate module, and neither a
  database SDK nor OTel may spread across providers.

## 2. Mental model

- **One canonical JSON envelope.** All data is read and written as the current
  `core/chat.Message` tagged wire. Migrating historical data is done explicitly
  by the application; no compatibility branch survives in the library.
- **Partitioned by conversation ID.** Each conversation has its own query path,
  so no operation scans across conversations.
- **Ordering comes from a monotonic sequence or list append, never a
  timestamp.** Under concurrency, timestamp ordering is not stable.
- **Schema initialization is an explicit switch.** Production usually migrates
  ahead of time and turns automatic table creation off.
- **A custom table name must pass SQL identifier validation.** This is the
  injection trust boundary.

## 3. Negative invariants

- Never build a cross-backend data migration tool. That is operations work.
- Never write a schema migration here. The caller migrates; this module only
  agrees on the shape.
- Never bring a database or observability dependency back into Core. A provider
  owns its own persistence mechanism, and OTel decorates explicitly at the
  composition root through the `otel` module.

## 4. Read before changing

- Changing `chat.Message` serialization forces every provider's local
  persistence boundary to follow, keeping its current wire tests.
- Adding a backend: create an independent module at `historystores/<backend>`,
  implement the real `core/history` capabilities, partition by conversation ID,
  and never import a sibling provider.
