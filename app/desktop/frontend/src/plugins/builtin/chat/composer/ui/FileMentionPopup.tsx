import { FloatingSurface, Icon, OptionRow, SectionLabel } from "@/ui";
import {
  MENTION_LISTBOX_ID,
  mentionOptionId,
} from "@/plugins/builtin/chat/composer/application/fileMentions";
import { useT } from "@/lib/i18n";

interface Props {
  items: string[];
  index: number;
  onPick: (path: string) => void;
  onHover: (i: number) => void;
}

// @file picker — a floating panel anchored above the composer. The selected row tracks
// keyboard ↑/↓ (driven by useFileMentions); hovering a row also selects it so click and
// key land on the same target. Basename emphasized, directory dimmed — the path reads as
// "name · where".
//
// A listbox of options, wired by hand: focus stays in the textarea (the caret has
// to keep blinking where the user is typing), so the selected row is announced
// through aria-activedescendant rather than by moving focus.
export function FileMentionPopup({ items, index, onPick, onHover }: Props) {
  const t = useT();
  return (
    <FloatingSurface
      id={MENTION_LISTBOX_ID}
      role="listbox"
      aria-label={t("composer.mention.heading")}
      className="absolute bottom-full left-2 right-2 z-1 mb-2 p-1"
    >
      <SectionLabel className="px-2.5 pb-1 pt-1.5">{t("composer.mention.heading")}</SectionLabel>
      {items.map((path, i) => {
        const slash = path.lastIndexOf("/");
        const dir = slash >= 0 ? path.slice(0, slash + 1) : "";
        const name = slash >= 0 ? path.slice(slash + 1) : path;
        return (
          <OptionRow
            key={path}
            id={mentionOptionId(i)}
            tabIndex={-1}
            selected={i === index}
            onMouseEnter={() => onHover(i)}
            onMouseDown={(e) => {
              // mousedown (not click) so the pick fires before the textarea blurs.
              e.preventDefault();
              onPick(path);
            }}
            className="grid-cols-[auto_1fr]"
          >
            <Icon name="filetext" size="sm" className="shrink-0 text-fg-muted" />
            <span className="truncate font-mono">
              <span className="text-fg-faint">{dir}</span>
              <span className="font-medium text-fg">{name}</span>
            </span>
          </OptionRow>
        );
      })}
    </FloatingSurface>
  );
}
