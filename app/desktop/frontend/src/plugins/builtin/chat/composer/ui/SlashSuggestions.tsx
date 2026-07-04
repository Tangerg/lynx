import { useMemo } from "react";
import { useT } from "@/lib/i18n";
import { useSlashCommands } from "@/plugins/sdk";

interface Props {
  value: string;
  onPick: (cmd: string) => void;
}

/**
 * Auto-suggest panel that appears when the composer value starts with "/".
 *
 * Commands come from the plugin registry; built-in hints live in the
 * `lyra.builtin.slash-hints` plugin. Clicking a row fills the composer
 * with the command + a trailing space — Enter on the composer still
 * submits the full typed text, so there's no implicit "pick first on
 * Enter" behavior.
 */
export function SlashSuggestions({ value, onPick }: Props) {
  const t = useT();
  const commands = useSlashCommands();

  const filtered = useMemo(() => {
    if (!value || !value.startsWith("/")) return [];
    const q = value.slice(1).toLowerCase();
    return commands
      .filter(({ cmd }) => cmd.slice(1).toLowerCase().startsWith(q))
      .sort((a, b) => a.cmd.localeCompare(b.cmd))
      .slice(0, 5);
  }, [value, commands]);

  if (filtered.length === 0) return null;

  return (
    <div className="mb-2 overflow-hidden rounded-[12px] bg-canvas p-1 shadow-[var(--shadow-popover)] animate-rise-in">
      <div className="px-2.5 pb-1 pt-1.5 font-mono text-[11px] font-semibold text-fg-faint">
        {t("composer.slash.heading")}
      </div>
      {filtered.map(({ cmd, spec }) => (
        <button
          key={cmd}
          type="button"
          onClick={() => onPick(`${cmd} `)}
          className="grid h-8 w-full grid-cols-[auto_1fr] items-center gap-2.5 rounded-md border-0 bg-transparent px-2.5 text-left text-[13px] text-fg-soft transition-colors hover:bg-fg/[0.06] hover:text-fg"
        >
          <code className="border-0 bg-transparent p-0 font-mono text-[12px] font-semibold text-accent">
            {cmd}
          </code>
          <span className="truncate text-[12px] text-fg-muted">{t(spec.description)}</span>
        </button>
      ))}
    </div>
  );
}
