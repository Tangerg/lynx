import type { ComposerImage, PastedText } from "@/plugins/builtin/chat/composer/public/attachments";
import type { IconName } from "@/ui";
import type { ComposerAttachmentSourceSpec } from "@/plugins/sdk";
import { Chip, Icon, MEDIA_OUTLINE, Tooltip } from "@/ui";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";

type AttachmentSource = ComposerAttachmentSourceSpec;

interface Props {
  sources: AttachmentSource[];
  images: ComposerImage[];
  pastes: PastedText[];
  onRemoveImage: (id: string) => void;
  onRemovePaste: (id: string) => void;
}

export function ComposerAttachments({
  sources,
  images,
  pastes,
  onRemoveImage,
  onRemovePaste,
}: Props) {
  return (
    <>
      <PluginAttachments sources={sources} />
      {images.length > 0 && (
        <div className="flex flex-wrap gap-2 pb-1 pt-1">
          {images.map((img) => (
            <ImageThumb key={img.id} image={img} onRemove={() => onRemoveImage(img.id)} />
          ))}
        </div>
      )}
      {pastes.length > 0 && (
        <div className="flex flex-wrap gap-1.5 pb-1 pt-1">
          {pastes.map((p) => (
            <PasteChip key={p.id} paste={p} onRemove={() => onRemovePaste(p.id)} />
          ))}
        </div>
      )}
    </>
  );
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
    <div className={cn("group relative h-14 w-14 overflow-hidden rounded-md", MEDIA_OUTLINE)}>
      <img
        src={`data:${image.mime};base64,${image.data}`}
        alt={image.name ?? ""}
        title={image.name}
        className="h-full w-full object-cover"
      />
      <button
        type="button"
        aria-label={t("composer.removeImage")}
        onClick={onRemove}
        className="absolute right-0.5 top-0.5 grid h-4 w-4 place-items-center rounded-full border-0 bg-black/55 text-white opacity-0 transition-opacity group-hover:opacity-100"
      >
        <Icon name="x" size={9} />
      </button>
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
      <span className="group inline-flex h-6 max-w-[220px] items-center gap-1.5 rounded-full bg-fg/[0.05] pl-2.5 pr-1.5 font-mono text-[11.5px] text-fg-muted">
        <Icon name="filetext" size={11} className="shrink-0 text-fg-faint" />
        <span className="truncate">{label}</span>
        <button
          type="button"
          aria-label={t("composer.paste.remove")}
          onClick={onRemove}
          className="grid h-4 w-4 shrink-0 place-items-center rounded-full border-0 bg-transparent text-fg-faint transition-colors hover:text-fg"
        >
          <Icon name="x" size={9} />
        </button>
      </span>
    </Tooltip>
  );
}
