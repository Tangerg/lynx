import type { ComposerImage, PastedText } from "@/plugins/builtin/chat/composer/public/attachments";
import type { IconName } from "@/ui";
import type { ComposerAttachmentSourceSpec } from "@/plugins/sdk";
import { AnimatePresence, motion } from "motion/react";
import { chipPresence } from "@/lib/motion";
import { Chip, Icon, IconButton, Tooltip } from "@/ui";
import { useT } from "@/lib/i18n";
import { draftMentions, removeMention } from "../application/draftContext";

type AttachmentSource = ComposerAttachmentSourceSpec;

interface Props {
  sources: AttachmentSource[];
  images: ComposerImage[];
  pastes: PastedText[];
  /** The draft, for the files it references. */
  value: string;
  onChange: (value: string) => void;
  onRemoveImage: (id: string) => void;
  onRemovePaste: (id: string) => void;
}

export function ComposerAttachments({
  sources,
  images,
  pastes,
  value,
  onChange,
  onRemoveImage,
  onRemovePaste,
}: Props) {
  return (
    <>
      <PluginAttachments sources={sources} />
      <DraftContext value={value} onChange={onChange} />
      {/* These arrive and leave because the USER put them there and took them away,
          which is the one thing presence animation is for.
          Presence only — no `layout`. `value` is a prop, so this whole subtree
          re-renders on every keystroke, and `layout` measures its element on every
          render: a chip in the composer would have cost a getBoundingClientRect and a
          projection pass per character typed. MessageStream records the same lesson
          one file over, for the same library, and I added it here anyway. */}
      {images.length > 0 && (
        <div className="flex flex-wrap gap-2 pb-1 pt-1">
          <AnimatePresence initial={false}>
            {images.map((img) => (
              <motion.div key={img.id} {...chipPresence}>
                <ImageThumb image={img} onRemove={() => onRemoveImage(img.id)} />
              </motion.div>
            ))}
          </AnimatePresence>
        </div>
      )}
      {pastes.length > 0 && (
        <div className="flex flex-wrap gap-1.5 pb-1 pt-1">
          <AnimatePresence initial={false}>
            {pastes.map((p) => (
              <motion.div key={p.id} {...chipPresence}>
                <PasteChip paste={p} onRemove={() => onRemovePaste(p.id)} />
              </motion.div>
            ))}
          </AnimatePresence>
        </div>
      )}
    </>
  );
}

/**
 * The files the draft references, as chips.
 *
 * An `@path` in the text is an attachment and needs a distinct visual identity.
 * Derived from the draft on every render rather
 * than tracked, because the text is what gets sent: a second list could disagree with
 * the message.
 */
function DraftContext({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const mentions = draftMentions(value);
  if (mentions.length === 0) return null;
  return (
    <div className="flex flex-wrap gap-1.5 pt-1 pb-0.5">
      <AnimatePresence initial={false}>
        {mentions.map((mention) => (
          <motion.div key={`${mention.start}:${mention.path}`} {...chipPresence}>
            <Chip
              icon="filetext"
              title={mention.path}
              onClose={() => onChange(removeMention(value, mention))}
            >
              {basename(mention.path)}
            </Chip>
          </motion.div>
        ))}
      </AnimatePresence>
    </div>
  );
}

/** The chip shows the filename; the whole path is the tooltip. A column of chips each
 *  reading `app/desktop/frontend/src/…` says nothing the others do not. */
function basename(path: string): string {
  const cut = path.lastIndexOf("/");
  return cut >= 0 ? path.slice(cut + 1) : path;
}

function PluginAttachments({ sources }: { sources: AttachmentSource[] }) {
  if (sources.length === 0) return null;
  return (
    <div className="flex flex-wrap gap-1.5 pb-0.5 pt-1">
      {sources.map((source) => (
        <SourceChips key={source.id} source={source} />
      ))}
    </div>
  );
}

// Each contributed source runs its hook inside its own component, so a buggy
// attachment source is isolated to that one chip group.
function SourceChips({ source }: { source: AttachmentSource }) {
  const items = source.useAttachments();
  return (
    <>
      {items.map((attachment) => (
        <Chip
          key={`${source.id}:${attachment.id ?? attachment.label}`}
          icon={(attachment.icon as IconName | undefined) ?? "file"}
          title={attachment.label}
        >
          {attachment.label}
        </Chip>
      ))}
    </>
  );
}

function ImageThumb({ image, onRemove }: { image: ComposerImage; onRemove: () => void }) {
  const t = useT();
  return (
    <div className="group relative h-14 w-14 overflow-hidden rounded-md media-edge">
      <img
        src={`data:${image.mime};base64,${image.data}`}
        alt={image.name ?? ""}
        title={image.name}
        className="h-full w-full object-cover"
      />
      <IconButton
        icon="x"
        size="xs"
        title={t("composer.removeImage")}
        aria-label={t("composer.removeImage")}
        onClick={onRemove}
        className="absolute right-0.5 top-0.5 rounded-full bg-media-scrim text-on-media opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
      />
    </div>
  );
}

function PasteChip({ paste, onRemove }: { paste: PastedText; onRemove: () => void }) {
  const t = useT();
  const preview = paste.text.slice(0, 160) + (paste.text.length > 160 ? "…" : "");
  const label =
    paste.lines > 1
      ? t("composer.paste.lines", { count: paste.lines })
      : t("composer.paste.chars", { count: paste.text.length });
  return (
    <Tooltip label={preview}>
      <span className="group inline-flex h-6 max-w-[220px] items-center gap-1.5 rounded-full bg-surface-2 pl-2.5 pr-1.5 font-mono text-ui-sm text-fg-muted">
        <Icon name="filetext" size="xs" className="shrink-0 text-fg-faint" />
        <span className="truncate">{label}</span>
        <IconButton
          icon="x"
          size="xs"
          title={t("composer.paste.remove")}
          aria-label={t("composer.paste.remove")}
          onClick={onRemove}
          className="shrink-0 rounded-full text-fg-faint hover:text-fg"
        />
      </span>
    </Tooltip>
  );
}
