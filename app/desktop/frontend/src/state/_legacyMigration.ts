// One-time migration of the legacy single-key `lyra.ui` persistence
// blob (from when there was a single useUIStore) into the per-domain
// keys the split stores now use.
//
// Runs idempotently: once `lyra.ui` is gone (either because we migrated
// it or it never existed), subsequent calls are no-ops. Every focused
// store calls this in its module init before Zustand's `persist`
// middleware hydrates, so user theme/layout/session state survives the
// refactor without anyone having to clear localStorage.

const LEGACY_KEY = "lyra.ui";

type LegacyState = Partial<{
  theme: string;
  accent: string;
  sidebarRail: boolean;
  activeSessionId: string;
  tabIds: string[];
}>;

/**
 * Read the old `lyra.ui` blob, fan out into the new per-domain keys,
 * delete the legacy key. Safe to call from any module init order.
 */
export function migrateLegacyUIStore(): void {
  if (typeof localStorage === "undefined") return;
  const raw = localStorage.getItem(LEGACY_KEY);
  if (!raw) return;

  let state: LegacyState = {};
  try {
    state = (JSON.parse(raw)?.state ?? {}) as LegacyState;
  } catch {
    // Corrupt JSON — drop the legacy key and start clean.
    localStorage.removeItem(LEGACY_KEY);
    return;
  }

  // Each fan-out target uses the same envelope shape Zustand's `persist`
  // middleware writes: `{ state: ..., version: ... }`. Version starts at
  // 1 for the new keys.
  if (state.theme !== undefined || state.accent !== undefined) {
    writeIfAbsent("lyra.theme", {
      theme: state.theme,
      accent: state.accent,
    });
  }
  if (state.sidebarRail !== undefined) {
    writeIfAbsent("lyra.layout", {
      sidebarRail: state.sidebarRail,
    });
  }
  if (state.activeSessionId !== undefined || state.tabIds !== undefined) {
    writeIfAbsent("lyra.session", {
      activeSessionId: state.activeSessionId,
      tabIds: state.tabIds,
    });
  }

  localStorage.removeItem(LEGACY_KEY);
}

// Don't overwrite a new key that already exists — if the user has
// already used the new build once, their newest preferences win over
// whatever was stuck in the legacy blob.
function writeIfAbsent(key: string, state: Record<string, unknown>): void {
  if (localStorage.getItem(key) !== null) return;
  localStorage.setItem(key, JSON.stringify({ state, version: 1 }));
}
