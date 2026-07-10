# Stateless RPC Migration

> Status: complete
> Owner: runtime protocol
> Started: 2026-07-10

## Goal

Move the Lyra Runtime Protocol from a connection-initialized shape to a stateless
JSON-RPC shape:

- No business method requires `runtime.initialize` first.
- Client identity, protocol version, and client capabilities travel with each
  relevant request as request metadata.
- Capability decisions are scoped to the request / run / subscription that uses
  them, not to the runtime process.
- Discovery remains available for clients that need server info or feature
  flags before choosing UI affordances.
- A network disconnect still detaches only the stream; it does not cancel the
  server-side run.

## Non-Goals

- Do not switch away from JSON-RPC 2.0.
- Do not replace streamable HTTP with WebSocket.
- Do not add a global notification stream unless a future multi-run push use
  case requires it.
- Do not keep legacy handshake shims. This project is still in the phase where
  breaking protocol changes should be made at the source.

## Target Shape

### Discovery

`runtime.discover` returns the same server-side information that HTTP
`GET /v2/info` exposes:

```ts
interface DiscoverResponse {
  protocolVersion: string;
  serverInfo: ServerInfo;
  capabilities: ServerCapabilities;
}
```

Discovery is optional. It is a normal RPC method, not a lifecycle transition.

### Request Metadata

Every JSON-RPC call may include `_meta` inside `params`:

```ts
interface RequestMeta {
  protocolVersion?: string;
  clientInfo?: ClientInfo;
  clientCapabilities?: ClientCapabilities;
}
```

The runtime strips `_meta` at the dispatch boundary before decoding typed
business params. This keeps individual method structs focused on business data
while allowing every transport to carry equivalent metadata.

### Capability Semantics

- `clientCapabilities.interruptTypes` gates HITL interrupts for the run being
  started / resumed.
- `clientCapabilities.events` and `optOutNotificationMethods` gate outbound
  stream events for the subscription that carries them.
- Absence of request metadata means "generic client": server must stay safe and
  avoid unanswerable interrupts.

## Plan

1. Document the migration target and progress here.
2. Replace `runtime.initialize` with `runtime.discover` in protocol types,
   dispatch, server, HTTP sidecar shape, and tests.
3. Add request metadata extraction at the dispatch boundary and expose it through
   context.
4. Move HITL capability selection from runtime-global initialize handling to the
   run start / resume call path.
5. Update desktop RPC types and methods so calls automatically attach `_meta`.
6. Update protocol docs and golden samples.
7. Run Go and frontend verification.

## Progress

| Step                    | Status | Notes                                                                   |
| ----------------------- | ------ | ----------------------------------------------------------------------- |
| 1. Migration doc        | done   | This file is the working checklist.                                     |
| 2. Replace initialize   | done   | Backend uses `runtime.discover`; dispatcher has no initialize gate.     |
| 3. Request metadata     | done   | Dispatch strips `params._meta` into context before typed decoding.      |
| 4. HITL scoping         | done   | Turn interrupt kinds are per-turn / per-rehydrate, not runtime-global.  |
| 5. Desktop RPC metadata | done   | RpcClient injects `params._meta`; bootstrap uses `runtime.discover`.    |
| 6. Docs and samples     | done   | API/TRANSPORT docs and golden samples use discovery + request metadata. |
| 7. Verification         | done   | Go, frontend typecheck/lint/test, format check, and golangci-lint pass. |

## Completed Checks

- `runtime.initialize` references are gone from runtime / frontend / canonical
  protocol docs; this migration note keeps the old name only to describe the
  change.
- Pre-discovery business dispatch is covered by HTTP tests: business methods do
  not require `runtime.discover`.
- `runtime.discover` and `/v2/info` read the same server capability snapshot.
- Stream methods carry `params._meta` without changing typed business params;
  dispatch strips metadata before decoding.

## Verification

- `GOWORK=/Users/tangerg/Desktop/lynx/go.work go test ./...` in
  `app/runtime`
- `GOWORK=/Users/tangerg/Desktop/lynx/go.work go vet ./...` in `app/runtime`
- `GOWORK=/Users/tangerg/Desktop/lynx/go.work golangci-lint run ./...` in
  `app/runtime`
- `npm run typecheck` in `app/desktop/frontend`
- `npm run lint` in `app/desktop/frontend`
- `npm run test` in `app/desktop/frontend`
- `npm run format:check` in `app/desktop/frontend`
