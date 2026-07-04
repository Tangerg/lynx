import { Icon } from "@/ui";
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
    <div className="absolute bottom-full left-2 right-2 z-10 mb-2 overflow-hidden rounded-[12px] bg-canvas p-1 shadow-[var(--shadow-popover)] animate-rise-in">
      <div className="px-2.5 pb-1 pt-1.5 font-mono text-[11px] font-semibold text-fg-faint">
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
              "grid h-8 w-full grid-cols-[auto_1fr] items-center gap-2.5 rounded-md border-0 bg-transparent px-2.5 text-left text-[13px] transition-colors",
              i === index ? "bg-fg/[0.06]" : "hover:bg-fg/[0.06]",
            )}
          >
            <Icon name="filetext" size={13} className="shrink-0 text-fg-muted" />
            <span className="truncate font-mono text-[12px]">
              <span className="text-fg-faint">{dir}</span>
              <span className="font-medium text-fg">{name}</span>
            </span>
          </button>
        );
      })}
    </div>
  );
}
