import { useMemo } from "react";
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
    <div className="mb-2 overflow-hidden rounded-xl border border-line bg-surface shadow-lg animate-rise-in">
      <div className="px-3.5 pb-1 pt-2 font-mono text-[11px] font-semibold text-fg-faint">
        Commands
      </div>
      {filtered.map(({ cmd, spec }) => (
        <button
          key={cmd}
          type="button"
          onClick={() => onPick(`${cmd} `)}
          className="grid w-full grid-cols-[auto_1fr] items-center gap-3 px-3.5 py-1.5 text-left text-inherit bg-transparent border-0 font-inherit transition-colors duration-150 hover:bg-surface-2"
        >
          <code className="bg-transparent p-0 font-mono text-[12.5px] font-semibold text-accent border-0">
            {cmd}
          </code>
          <span className="text-[12.5px] text-fg-muted">{spec.description}</span>
        </button>
      ))}
    </div>
  );
}
