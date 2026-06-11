// Built-in plugin: the app's ONE workspace.subscribe consumer (AUX_API §3).
//
// Opens a single app-wide workspace event stream once the handshake
// advertises the method, and translates the lossy "changed" signals into
// react-query invalidations — panels refetch instead of polling staleTime.
//
// Channel semantics (AUX_API §3.1/§3.2):
// - connection-scoped, no replay: when the stream ends (network drop,
//   runtime restart, graceful EOS) we resubscribe with backoff;
// - every (re)subscribe is an implicit `resync` → invalidate everything
//   BEFORE panels refetch ("subscribe first, then fetch" — no lost window);
// - per-event invalidation is local (one cache key per domain); the bare
//   `resync` event is the lost-events fallback → invalidate everything.

import type { WorkspaceEvent } from "@/rpc";
import { WORKSPACE_SUBSCRIBE_METHOD } from "@/rpc/transport";
import { queryClient } from "@/lib/data/queryClient";
import { DIFF_KEY, FILES_CHANGED_KEY, MCP_SERVERS_KEY, SKILLS_KEY } from "@/lib/data/queries";
import { getContainer } from "@/main/container";
import { definePlugin } from "@/plugins/sdk";
import { useRuntimeStore } from "@/state/runtimeStore";

const RECONNECT_BASE_MS = 1_000;
const RECONNECT_CAP_MS = 30_000;

function invalidate(...keys: string[]): void {
  for (const key of keys) void queryClient.invalidateQueries({ queryKey: [key] });
}

/** Everything was potentially missed — refetch all cached panel data. */
function invalidateAll(): void {
  void queryClient.invalidateQueries();
}

function handle(ev: WorkspaceEvent): void {
  switch (ev.type) {
    case "files.changed":
      invalidate(FILES_CHANGED_KEY, DIFF_KEY);
      return;
    case "skills.changed":
      invalidate(SKILLS_KEY);
      return;
    case "mcp.serverChanged":
      invalidate(MCP_SERVERS_KEY);
      return;
    case "resync":
      invalidateAll();
      return;
    default:
      // Forward-compat: unknown event types are fine to ignore — anything we
      // don't understand we also don't cache by that name. Sessions are
      // deliberately absent above: session state converges through run
      // streams, not this channel.
      return;
  }
}

function delay(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const t = setTimeout(done, ms);
    function done(): void {
      clearTimeout(t);
      signal.removeEventListener("abort", done);
      resolve();
    }
    signal.addEventListener("abort", done, { once: true });
  });
}

async function subscribeLoop(signal: AbortSignal): Promise<void> {
  let attempt = 0;
  while (!signal.aborted) {
    try {
      // One recursive watch on the serve-dir root drives files-changed/diff
      // refresh (paths come debounced + lossy, AUX_API §3). Bare subscription
      // (skills/mcp/resync) works regardless of the fileWatch capability.
      const fileWatch = useRuntimeStore.getState().capabilities?.features.fileWatch === true;
      const { events } = await getContainer()
        .client()
        .workspace.subscribe(
          fileWatch ? { watches: [{ watchId: "workspace-root" }] } : undefined,
          signal,
        );
      attempt = 0;
      // (Re)connected = implicit resync: anything that changed while we were
      // dark is unknown, so refetch before trusting the cache again.
      invalidateAll();
      for await (const ev of events) handle(ev);
    } catch (err) {
      if (!signal.aborted) console.warn("[workspace-events] subscribe failed:", err);
    }
    if (signal.aborted) return;
    // Exponential backoff, capped — covers both call failures and stream
    // ends (the for-await exits when the POST stream dies, see STREAM_DOWN).
    await delay(Math.min(RECONNECT_BASE_MS * 2 ** attempt, RECONNECT_CAP_MS), signal);
    attempt += 1;
  }
}

export default definePlugin({
  name: "lyra.builtin.workspace-events",
  version: "1.0.0",
  setup() {
    const controller = new AbortController();
    let started = false;

    const startIfAdvertised = (): void => {
      if (started || controller.signal.aborted) return;
      const caps = useRuntimeStore.getState().capabilities;
      if (!caps?.streamingMethods?.includes(WORKSPACE_SUBSCRIBE_METHOD)) return;
      started = true;
      void subscribeLoop(controller.signal);
    };

    // The handshake (bootstrap plugin) may land before or after this plugin's
    // setup — check now, and watch the store for the capabilities arriving.
    startIfAdvertised();
    const unsub = useRuntimeStore.subscribe(startIfAdvertised);

    return () => {
      unsub();
      controller.abort();
    };
  },
});
