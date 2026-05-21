import { useMemo } from "react";
import { useSlashCommands } from "@/plugins/sdk";

type Props = {
  value: string;
  onPick: (cmd: string) => void;
};

/**
 * Auto-suggest panel that appears when the composer value starts with "/".
 *
 * Commands come from the plugin registry; built-in hints live in the
 * `lyra.builtin.slash-hints` plugin. No hard-coded list here anymore.
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
    <div className="slash-panel">
      <div className="slash-head">Commands</div>
      {filtered.map(({ cmd, spec }, i) => (
        <button
          key={cmd}
          type="button"
          className="slash-row"
          onClick={() => onPick(`${cmd} `)}
        >
          <code className="slash-cmd">{cmd}</code>
          <span className="slash-desc">{spec.description}</span>
          {i === 0 && <span className="slash-hint">↵</span>}
        </button>
      ))}
    </div>
  );
}
