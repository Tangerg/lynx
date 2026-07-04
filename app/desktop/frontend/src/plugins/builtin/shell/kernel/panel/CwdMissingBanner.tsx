// CwdMissingBanner — surfaces the cwdMissing degrade state (API.md §4.1):
// the session's working directory is gone from disk, so the runtime degrades
// the agent to plain chat (no filesystem tools) until the folder comes back
// or the session is relocated. The relocate entry (sessions.update cwd) is
// gated on features.relocate; without it the banner is informational only.
//
// Sits with RunErrorBanner above the message stream. Warning-toned, not
// negative: the session still works, just degraded.

import { useRef, useState } from "react";
import { FIELD_CLASSES, Icon } from "@/ui";
import { cn } from "@/lib/utils";
import { useActiveSession, useRelocateSession } from "@/plugins/builtin/agent/public/session";
import { BannerAction } from "./BannerAction";
import { useT } from "@/lib/i18n";
import { useServerFeature } from "@/state/runtimeStore";

export function CwdMissingBanner() {
  const t = useT();
  const session = useActiveSession();
  const relocateEnabled = useServerFeature("relocate");
  const relocate = useRelocateSession();
  const [editing, setEditing] = useState(false);
  const [path, setPath] = useState("");
  const [busy, setBusy] = useState(false);
  // Synchronous re-entrancy latch — `busy` state lags a render, so Enter + an
  // immediate Apply click in one tick would otherwise both fire relocate.
  const submitting = useRef(false);

  if (!session?.cwdMissing) return null;

  const submit = async (): Promise<void> => {
    const next = path.trim();
    if (!next || submitting.current) return;
    submitting.current = true;
    setBusy(true);
    const ok = await relocate(session.id, next);
    submitting.current = false;
    setBusy(false);
    if (ok) {
      setEditing(false);
      setPath("");
    }
  };

  return (
    <div
      role="alert"
      className="mx-4 mt-2.5 mb-1 grid grid-cols-[auto_1fr] items-start gap-2.5 rounded-[12px] bg-warning/10 px-4 py-3 font-sans text-fg"
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
                    if (e.nativeEvent.isComposing) return; // let the IME commit its candidate
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
                  className={cn(FIELD_CLASSES, "h-6.5 w-72 max-w-full px-2 text-fg")}
                />
                <BannerAction
                  label={busy ? "…" : t("cwdMissing.action.apply")}
                  onClick={() => void submit()}
                  primary
                  tone="warning"
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
                tone="warning"
              />
            )}
          </div>
        )}
      </div>
    </div>
  );
}
