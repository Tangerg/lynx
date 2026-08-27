import { useMemo } from "react";
import { useT } from "@/lib/i18n";
import { useSlashCommands } from "@/plugins/sdk";
import { FloatingSurface, OptionRow, SectionLabel } from "@/ui";

interface Props {
  value: string;
  onPick: (cmd: string) => void;
}

/**
 * Auto-suggest panel that appears when the composer value starts with "/".
 *
 * Commands come from the plugin registry; built-in hints live in the
 * `scopeapp.builtin.slash-hints` plugin. Clicking a row fills the composer
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
    <FloatingSurface className="mb-2 p-1">
      <SectionLabel className="px-2.5 pb-1 pt-1.5">{t("composer.slash.heading")}</SectionLabel>
      {filtered.map(({ cmd, spec }) => (
        <OptionRow key={cmd} onClick={() => onPick(`${cmd} `)} className="grid-cols-[auto_1fr]">
          <code className="border-0 bg-transparent p-0 font-mono font-semibold text-accent">
            {cmd}
          </code>
          <span className="truncate text-fg-muted">{t(spec.description)}</span>
        </OptionRow>
      ))}
    </FloatingSurface>
  );
}
