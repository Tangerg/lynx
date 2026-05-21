// Built-in plugin: Cmd+K command palette.
//
// Subscribes the global Mod+K shortcut, mounts an overlay on app.overlay,
// and renders all plugin-registered commands. Search filters by label +
// description + keywords. Up/Down navigate; Enter runs.

import { useEffect, useMemo, useRef, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { create } from "zustand";
import { Icon, type IconName } from "@/components/common";
import { popIn, swift } from "@/lib/motion";
import { definePlugin, evalWhen, reportPluginError, useCommands, usePluginStore, type CommandSpec } from "@/plugins/sdk";
import { useWhenContext } from "@/state/useWhenContext";

// ---------- store ---------------------------------------------------------

type PaletteState = {
  open: boolean;
  query: string;
  setOpen: (open: boolean) => void;
  setQuery: (q: string) => void;
  toggle: () => void;
};

const usePaletteStore = create<PaletteState>((set) => ({
  open: false,
  query: "",
  setOpen: (open) => set({ open, query: "" }),
  setQuery: (query) => set({ query }),
  toggle: () => set((s) => ({ open: !s.open, query: "" })),
}));

// ---------- filtering -----------------------------------------------------

function matches(cmd: CommandSpec, q: string): boolean {
  if (!q) return true;
  const haystack = [
    cmd.label,
    cmd.description ?? "",
    cmd.group ?? "",
    ...(cmd.keywords ?? []),
  ].join(" ").toLowerCase();
  return haystack.includes(q.toLowerCase());
}

// ---------- palette UI ----------------------------------------------------

function CommandPalette() {
  const open = usePaletteStore((s) => s.open);
  const query = usePaletteStore((s) => s.query);
  const setOpen = usePaletteStore((s) => s.setOpen);
  const setQuery = usePaletteStore((s) => s.setQuery);
  const commands = useCommands();
  const whenCtx = useWhenContext();

  const visible = useMemo(
    () => commands.filter((c) => !c.when || evalWhen(c.when, whenCtx)),
    [commands, whenCtx],
  );
  const filtered = useMemo(
    () => visible.filter((c) => matches(c, query)),
    [visible, query],
  );
  const [activeIdx, setActiveIdx] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  // Reset selection when filtered list changes.
  useEffect(() => { setActiveIdx(0); }, [query, commands]);

  // Autofocus the input on open.
  useEffect(() => {
    if (open) inputRef.current?.focus();
  }, [open]);

  const run = (cmd: CommandSpec) => {
    setOpen(false);
    void Promise.resolve(cmd.run()).catch((err) => {
      // eslint-disable-next-line no-console
      console.error(`[plugin] command ${cmd.id} threw:`, err);
      const owner = usePluginStore.getState().commands.get(cmd.id)?.pluginName ?? "unknown";
      reportPluginError(owner, "command", err, `command: ${cmd.id}`);
    });
  };

  return (
    <AnimatePresence>
      {open && (
        <motion.div
          className="cmdk-backdrop"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={swift}
          onClick={() => setOpen(false)}
        >
          <motion.div
            className="cmdk"
            {...popIn}
            onClick={(e) => e.stopPropagation()}
            role="dialog"
            aria-modal="true"
          >
            <div className="cmdk-search">
              <Icon name="search" size={14} />
              <input
                ref={inputRef}
                className="cmdk-input"
                placeholder="Type a command…"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "ArrowDown") {
                    e.preventDefault();
                    setActiveIdx((i) => Math.min(filtered.length - 1, i + 1));
                  } else if (e.key === "ArrowUp") {
                    e.preventDefault();
                    setActiveIdx((i) => Math.max(0, i - 1));
                  } else if (e.key === "Enter") {
                    e.preventDefault();
                    const target = filtered[activeIdx];
                    if (target) run(target);
                  } else if (e.key === "Escape") {
                    e.preventDefault();
                    e.stopPropagation();
                    setOpen(false);
                  }
                }}
              />
              <span className="cmdk-hint">esc</span>
            </div>
            <div className="cmdk-list">
              {filtered.length === 0 && (
                <div className="cmdk-empty">No commands match</div>
              )}
              {filtered.map((cmd, i) => (
                <button
                  key={cmd.id}
                  className={`cmdk-item ${i === activeIdx ? "active" : ""}`}
                  onMouseEnter={() => setActiveIdx(i)}
                  onClick={() => run(cmd)}
                >
                  {cmd.icon && <Icon name={cmd.icon as IconName} size={14} />}
                  <div className="cmdk-item-body">
                    <div className="cmdk-item-label">{cmd.label}</div>
                    {cmd.description && (
                      <div className="cmdk-item-sub">{cmd.description}</div>
                    )}
                  </div>
                  {cmd.group && <span className="cmdk-item-group">{cmd.group}</span>}
                  {cmd.shortcut && <span className="cmdk-item-shortcut">{cmd.shortcut}</span>}
                </button>
              ))}
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}

// ---------- plugin --------------------------------------------------------

export default definePlugin({
  name: "lyra.builtin.command-palette",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("app.overlay", {
      id: "command-palette",
      order: 10, // above settings (0), below toaster (100)
      component: CommandPalette,
    });

    host.shortcuts.register({
      key: "Mod+K",
      description: "Open the command palette",
      // The palette swallows typing once open, so we want it to fire even
      // when an input is focused — that's the whole point of Cmd+K.
      allowInInputs: true,
      handler: (e) => {
        e.preventDefault();
        usePaletteStore.getState().toggle();
      },
    });
  },
});
