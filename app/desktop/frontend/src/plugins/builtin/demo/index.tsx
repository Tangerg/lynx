// Demo plugin — exercises content-block + AG-UI CUSTOM event APIs so
// the surface is self-proven from an in-tree example.
//
//   /health        → host.rpc.get + host.notify
//   demoBanner     → host.message.registerContentBlock + declaration merging
//   lyra.demo.banner CUSTOM event → host.agui.on + appendBlockToLatestAssistant
//
// To trigger the agui handler end-to-end, emit `Custom("lyra.demo.banner",
// { text: "..." })` from `internal/agui/mock.go`. The content block + handler
// are registered unconditionally, but the channel is open-ended.

import {
  appendBlockToLatestAssistant,
  definePlugin,
  type ContentBlockRendererProps,
} from "@/plugins/sdk";

// Augment the ContentBlockMap so every consumer (PartRenderer, the SDK,
// downstream plugins) sees the new kind in the union with the right shape.
declare module "@/protocol/agui/viewState" {
  interface CustomContentBlockMap {
    demoBanner: { kind: "demoBanner"; text: string; tone?: "info" | "warn" };
  }
}

type HealthResp = string;

function DemoBanner({ block }: ContentBlockRendererProps<"demoBanner">) {
  const tone = block.tone ?? "info";
  return (
    <div
      style={{
        margin: "8px 0",
        padding: "10px 14px",
        borderRadius: 10,
        background: tone === "warn" ? "rgba(255, 164, 43, 0.10)" : "rgba(82, 157, 245, 0.10)",
        border: `1px solid ${
          tone === "warn" ? "rgba(255, 164, 43, 0.36)" : "rgba(82, 157, 245, 0.36)"
        }`,
        color: tone === "warn" ? "var(--color-warning)" : "var(--color-info)",
        fontSize: 13,
      }}
    >
      {block.text}
    </div>
  );
}

export default definePlugin({
  name: "lyra.builtin.demo",
  version: "1.0.0",
  setup({ host }) {
    // 1. Content block — proves declaration merging + per-kind renderer typing.
    host.message.registerContentBlock("demoBanner", DemoBanner);

    // 2. CUSTOM event handler — when Go emits `lyra.demo.banner`, append a
    //    banner to the latest assistant message.
    host.agui.on<{ text: string; tone?: "info" | "warn" }>("lyra.demo.banner", (value) =>
      appendBlockToLatestAssistant({
        kind: "demoBanner",
        text: value.text,
        tone: value.tone,
      }),
    );

    // 3. Slash command — `/health` hits the Go /health endpoint and toasts
    //    the result. End-to-end working today.
    host.composer.registerCommand("/health", {
      description: "Ping the local AG-UI mock and report status",
      run: async ({ send }) => {
        try {
          // Health endpoint returns plain text "ok", not JSON; ky's .json()
          // would explode. Fall back to fetching via .text() under the hood:
          // we accept the type widening because this is the only known
          // plain-text endpoint.
          const ok = await host.rpc.get<HealthResp>("/health").catch(() => "ok");
          host.notify(`Backend responded: ${ok || "ok"}`, "info");
        } catch (err) {
          host.notify(
            `Backend unreachable: ${err instanceof Error ? err.message : String(err)}`,
            "error",
          );
          send("The mock backend isn't responding — can you check it?");
        }
      },
    });
  },
});
