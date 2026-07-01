import { Icon } from "@/components/common";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";

interface Props {
  items: string[];
  index: number;
  onPick: (path: string) => void;
  onHover: (i: number) => void;
}

// @file picker — a floating panel anchored above the composer (mirrors
// SlashSuggestions' look). The selected row tracks keyboard ↑/↓ (driven by
// useFileMentions); hovering a row also selects it so click and key land on the
// same target. Basename emphasized, directory dimmed — the path reads as
// "name · where".
export function FileMentionPopup({ items, index, onPick, onHover }: Props) {
  const t = useT();
  return (
    <div className="absolute bottom-full left-2 right-2 z-10 mb-2 overflow-hidden rounded-lg border-0 bg-surface shadow-[var(--shadow-popover)] animate-rise-in">
      <div className="px-3.5 pb-1 pt-2 font-mono text-[11px] font-semibold text-fg-faint">
        {t("composer.mention.heading")}
      </div>
      {items.map((path, i) => {
        const slash = path.lastIndexOf("/");
        const dir = slash >= 0 ? path.slice(0, slash + 1) : "";
        const name = slash >= 0 ? path.slice(slash + 1) : path;
        return (
          <button
            key={path}
            type="button"
            onMouseEnter={() => onHover(i)}
            onMouseDown={(e) => {
              // mousedown (not click) so the pick fires before the textarea blurs.
              e.preventDefault();
              onPick(path);
            }}
            className={cn(
              "grid w-full grid-cols-[auto_1fr] items-center gap-2.5 border-0 bg-transparent px-3.5 py-1.5 text-left font-inherit text-inherit transition-colors duration-150",
              i === index ? "bg-surface-2" : "hover:bg-surface-2",
            )}
          >
            <Icon name="filetext" size={12} className="shrink-0 text-fg-faint" />
            <span className="truncate font-mono text-[12.5px]">
              <span className="text-fg-faint">{dir}</span>
              <span className="font-semibold text-fg">{name}</span>
            </span>
          </button>
        );
      })}
    </div>
  );
}
