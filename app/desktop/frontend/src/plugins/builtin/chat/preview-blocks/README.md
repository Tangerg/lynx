# preview-blocks — content blocks awaiting backend wiring

These three content-block kinds have finished UI but the v2 Runtime fold
(`plugins/builtin/agent/application/fold`) **does not emit them yet**. They live
here, isolated, so the kernel never grows around features that aren't wired —
and so this whole folder can be kept or deleted as one unit.

| Block        | UI                    | Future feature       |
| ------------ | --------------------- | -------------------- |
| `search`     | `SearchResults.tsx`   | web-search tool      |
| `code`       | core `ShikiCodeBlock` | standalone edit tool |
| `checkpoint` | `Checkpoint.tsx`      | run checkpoints      |

## How it stays removable

- The kinds are declared in `viewBlocks.ts` via **`CustomContentBlockMap`
  augmentation**, NOT in the core `BuiltinContentBlockMap`. So `ContentBlock`
  only carries them while this folder exists.
- The renderers register through the normal `host.message.registerContentBlock`
  path; `code` reuses the shared core `ShikiCodeBlock` (also used by markdown
  fences), so deleting this folder does not take that component out.
- `search` also contributes the per-message **citation source**
  (`MESSAGE_CITATION_SOURCE`) that feeds the `[n]` markers. The kernel
  (`MessageBlock`) gathers sources generically and never names `search`, so
  removing this folder makes citations cleanly empty.

## To wire one up

Make the agent fold emit the block (a `toolCall` → `search`/`code`
projection, or a dedicated item type) and add it to the bootstrap
`CLIENT_CAPABILITIES.events` if it rides a new event.

## To remove

Delete this folder and drop its entry from
`plugins/builtin/index.ts`. Nothing else references it.
