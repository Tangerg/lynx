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
import { asSessionId } from "@/rpc";
import { WORKSPACE_SUBSCRIBE_METHOD } from "@/rpc/transport";
import { queryClient } from "@/lib/data/queryClient";
import {
  DIFF_KEY,
  FILES_CHANGED_KEY,
  MCP_SERVERS_KEY,
  MCP_TOOLS_KEY,
  SESSIONS_KEY,
  SKILLS_KEY,
} from "@/lib/data/queries";
import { getContainer } from "@/main/container";
import { definePlugin } from "@/plugins/sdk";
import { useRuntimeStore } from "@/state/runtimeStore";
import { useSessionStore } from "@/state/sessionStore";

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
      // Reconnect hot-swaps the tool set, so the expanded detail refetches too.
      invalidate(MCP_SERVERS_KEY, MCP_TOOLS_KEY);
      return;
    case "resync":
      // Watched cwd's git state changed (commit / stage / checkout / merge)
      // OR the lossy channel dropped events — either way the contract says
      // refetch (AUX_API §3.2). Full invalidation: only mounted panels
      // actually refetch, so the cost tracks what's visible.
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

// The cwd the current subscription watches, and the per-iteration abort
// that lets a cwd change close the stream for an immediate, backoff-free
// resubscribe (changing the watch set = close + resubscribe, AUX_API §3.1).
let watchCwd: string | undefined;
let iterAbort: AbortController | null = null;
let resubscribe = false;

/** Point the git-state watch at the active session's cwd. The sessions list
 *  cache is tried first; a cache miss falls back to one sessions.get. A
 *  change closes the current stream — the loop reopens it with the new
 *  watch immediately (no backoff). */
function retargetWatch(): void {
  const id = useSessionStore.getState().activeSessionId;
  if (!id) return; // keep the last known cwd — better than dropping to serve dir
  const cached = queryClient
    .getQueryData<{ id: string; cwd?: string }[]>([SESSIONS_KEY])
    ?.find((s) => s.id === id)?.cwd;
  const resolved = cached
    ? Promise.resolve(cached)
    : getContainer()
        .client()
        .sessions.get(asSessionId(id))
        .then((sess) => sess.cwd)
        .catch(() => undefined);
  void Promise.resolve(resolved).then((cwd) => {
    if (cwd === undefined || cwd === watchCwd) return;
    watchCwd = cwd;
    resubscribe = true;
    iterAbort?.abort();
  });
}

async function subscribeLoop(signal: AbortSignal): Promise<void> {
  let attempt = 0;
  while (!signal.aborted) {
    // Per-iteration controller so a watch retarget can end JUST this stream
    // while the outer signal still owns plugin-lifetime teardown.
    const iter = new AbortController();
    iterAbort = iter;
    const onOuterAbort = () => iter.abort();
    signal.addEventListener("abort", onOuterAbort, { once: true });
    try {
      // One watch on the ACTIVE SESSION's cwd = git-state monitoring
      // (AUX_API §3.1 watch model — NOT recursive file watching): git
      // changes arrive as debounced `resync`, the agent's own edits as
      // precise files.changed. Bare subscription (skills/mcp) works
      // regardless of fileWatch; cwd omitted = serve dir.
      const fileWatch = useRuntimeStore.getState().capabilities?.features.fileWatch === true;
      const { events } = await getContainer()
        .client()
        .workspace.subscribe(
          fileWatch ? { watches: [{ watchId: "active-session", cwd: watchCwd }] } : undefined,
          iter.signal,
        );
      attempt = 0;
      // (Re)connected = implicit resync: anything that changed while we were
      // dark is unknown, so refetch before trusting the cache again.
      invalidateAll();
      for await (const ev of events) handle(ev);
    } catch (err) {
      if (!signal.aborted && !resubscribe) {
        console.warn("[workspace-events] subscribe failed:", err);
      }
    } finally {
      signal.removeEventListener("abort", onOuterAbort);
      if (iterAbort === iter) iterAbort = null;
    }
    if (signal.aborted) return;
    if (resubscribe) {
      // Intentional close (watch retarget) — reopen immediately.
      resubscribe = false;
      attempt = 0;
      continue;
    }
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
    const unsubRuntime = useRuntimeStore.subscribe(startIfAdvertised);
    // Follow the active session: its cwd is what the git-state watch should
    // point at (the serve dir may be anywhere — BACKEND_API_REFERENCE §5).
    retargetWatch();
    const unsubSession = useSessionStore.subscribe(retargetWatch);

    return () => {
      unsubRuntime();
      unsubSession();
      controller.abort();
    };
  },
});
