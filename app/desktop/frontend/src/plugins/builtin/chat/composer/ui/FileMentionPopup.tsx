import { Icon, Pressable } from "@/ui";
import {
  MENTION_LISTBOX_ID,
  mentionOptionId,
} from "@/plugins/builtin/chat/composer/application/fileMentions";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";

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
//
// A listbox of options, wired by hand: focus stays in the textarea (the caret has
// to keep blinking where the user is typing), so the selected row is announced
// through aria-activedescendant rather than by moving focus.
export function FileMentionPopup({ items, index, onPick, onHover }: Props) {
  const t = useT();
  return (
    <div
      id={MENTION_LISTBOX_ID}
      role="listbox"
      aria-label={t("composer.mention.heading")}
      className="absolute bottom-full left-2 right-2 z-10 mb-2 overflow-hidden rounded-md bg-canvas p-1 shadow-[var(--shadow-popover)] animate-rise-in"
    >
      <div className="px-2.5 pb-1 pt-1.5 font-mono text-ui-sm font-semibold text-fg-faint">
        {t("composer.mention.heading")}
      </div>
      {items.map((path, i) => {
        const slash = path.lastIndexOf("/");
        const dir = slash >= 0 ? path.slice(0, slash + 1) : "";
        const name = slash >= 0 ? path.slice(slash + 1) : path;
        return (
          <Pressable
            key={path}
            id={mentionOptionId(i)}
            type="button"
            role="option"
            tabIndex={-1}
            aria-selected={i === index}
            onMouseEnter={() => onHover(i)}
            onMouseDown={(e) => {
              // mousedown (not click) so the pick fires before the textarea blurs.
              e.preventDefault();
              onPick(path);
            }}
            className={cn(
              "grid h-8 w-full grid-cols-[auto_1fr] items-center gap-2.5 rounded-md border-0 bg-transparent px-2.5 text-left text-ui-lg transition-colors",
              i === index ? "bg-selected" : "hover:bg-hover",
            )}
          >
            <Icon name="filetext" size="sm" className="shrink-0 text-fg-muted" />
            <span className="truncate font-mono text-ui-md">
              <span className="text-fg-faint">{dir}</span>
              <span className="font-medium text-fg">{name}</span>
            </span>
          </Pressable>
        );
      })}
    </div>
  );
}
