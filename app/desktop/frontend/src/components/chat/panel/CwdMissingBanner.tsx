// CwdMissingBanner — surfaces the cwdMissing degrade state (API.md §4.1):
// the session's working directory is gone from disk, so the runtime degrades
// the agent to plain chat (no filesystem tools) until the folder comes back
// or the session is relocated. The relocate entry (sessions.update cwd) is
// gated on features.relocate; without it the banner is informational only.
//
// Sits with RunErrorBanner above the message stream. Warning-toned, not
// negative: the session still works, just degraded.

import { useState } from "react";
import { Icon } from "@/components/common";
import { useSessions } from "@/lib/data/queries";
import { useRelocateSession } from "@/lib/agent/useRelocateSession";
import { useT } from "@/lib/i18n";
import { useServerFeature } from "@/state/runtimeStore";
import { useSessionStore } from "@/state/sessionStore";

export function CwdMissingBanner() {
  const t = useT();
  const activeSessionId = useSessionStore((s) => s.activeSessionId);
  const { data: sessions } = useSessions();
  const relocateEnabled = useServerFeature("relocate");
  const relocate = useRelocateSession();
  const [editing, setEditing] = useState(false);
  const [path, setPath] = useState("");
  const [busy, setBusy] = useState(false);

  const session = sessions?.find((s) => s.id === activeSessionId);
  if (!session?.cwdMissing) return null;

  const submit = async (): Promise<void> => {
    const next = path.trim();
    if (!next || busy) return;
    setBusy(true);
    const ok = await relocate(session.id, next);
    setBusy(false);
    if (ok) {
      setEditing(false);
      setPath("");
    }
  };

  return (
    <div
      role="alert"
      className="mx-4 mt-2.5 mb-1 grid grid-cols-[auto_1fr] items-start gap-2.5 rounded-lg border border-warning/35 bg-warning/12 px-3 py-2.5 font-sans text-fg"
    >
      <Icon name="alert" size={14} className="mt-0.5 text-warning" />
      <div className="min-w-0">
        <div className="mb-0.5 text-[13px] font-semibold text-warning">{t("cwdMissing.title")}</div>
        <div className="text-[13px] text-fg-soft break-words">
          <code className="font-mono text-[12px]">{session.cwd}</code> · {t("cwdMissing.body")}
        </div>
        {relocateEnabled && (
          <div className="mt-2">
            {editing ? (
              <div className="flex items-center gap-1.5">
                <input
                  type="text"
                  value={path}
                  onChange={(e) => setPath(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") void submit();
                    if (e.key === "Escape") setEditing(false);
                  }}
                  placeholder={t("cwdMissing.placeholder")}
                  aria-label={t("cwdMissing.placeholder")}
                  disabled={busy}
                  spellCheck={false}
                  // The input appears from an explicit "Relocate" click, so
                  // focusing it is the expected continuation, not a steal.
                  // eslint-disable-next-line jsx-a11y/no-autofocus
                  autoFocus
                  className="h-6.5 w-72 max-w-full rounded-md bg-canvas px-2 font-mono text-[12px] text-fg outline-none focus:ring-1 focus:ring-accent/40"
                />
                <BannerAction
                  label={busy ? "…" : t("cwdMissing.action.apply")}
                  onClick={() => void submit()}
                  primary
                />
                <BannerAction
                  label={t("cwdMissing.action.cancel")}
                  onClick={() => setEditing(false)}
                />
              </div>
            ) : (
              <BannerAction
                label={t("cwdMissing.action.relocate")}
                onClick={() => {
                  setPath(session.cwd ?? "");
                  setEditing(true);
                }}
                primary
              />
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function BannerAction({
  label,
  onClick,
  primary,
}: {
  label: string;
  onClick: () => void;
  primary?: boolean;
}) {
  const focusRing =
    "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent";
  return (
    <button
      type="button"
      onClick={onClick}
      className={
        primary
          ? `inline-flex h-6 items-center rounded-md border border-warning/40 bg-warning/15 px-2 font-sans text-[11.5px] font-semibold text-warning transition-colors hover:bg-warning/25 ${focusRing}`
          : `inline-flex h-6 items-center rounded-md border border-line-soft bg-transparent px-2 font-sans text-[11.5px] text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg ${focusRing}`
      }
    >
      {label}
    </button>
  );
}
