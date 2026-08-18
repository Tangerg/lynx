import { useCallback, useEffect, useState, type MouseEventHandler, type ReactElement } from "react";
import { useT } from "@/lib/i18n";
import { IconButton, LightboxDialog } from "@/ui";
import { MESSAGE_CONTENT_SELECTOR } from "./messageContent";

const PREVIEW_TRIGGER_ATTR = "data-message-image-preview-trigger";

interface GalleryItem {
  src: string;
  alt: string;
  title?: string;
  width?: number;
  height?: number;
}

interface GalleryState {
  items: GalleryItem[];
  index: number;
}

interface TriggerProps {
  "data-message-image-preview-trigger": "true";
  onClick: MouseEventHandler<HTMLButtonElement>;
}

interface Props {
  item: GalleryItem;
  titleFallback: string;
  trigger: (props: TriggerProps) => ReactElement;
}

function numericAttribute(image: HTMLImageElement, name: "width" | "height") {
  const value = image.getAttribute(name);
  if (!value) return undefined;
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined;
}

function galleryRoot(trigger: HTMLButtonElement) {
  return trigger.closest(MESSAGE_CONTENT_SELECTOR) ?? trigger.closest(".md");
}

function collectGallery(trigger: HTMLButtonElement, fallback: GalleryItem): GalleryState {
  const root = galleryRoot(trigger);
  if (!root) return { items: [fallback], index: 0 };

  const messageRoot = root.matches(MESSAGE_CONTENT_SELECTOR) ? root : null;
  const markdownRoot = messageRoot ? null : root;
  const triggers = Array.from(
    root.querySelectorAll<HTMLButtonElement>(`button[${PREVIEW_TRIGGER_ATTR}="true"]`),
  ).filter((candidate) =>
    messageRoot
      ? candidate.closest(MESSAGE_CONTENT_SELECTOR) === messageRoot
      : candidate.closest(".md") === markdownRoot,
  );

  const items: GalleryItem[] = [];
  let index = 0;
  for (const candidate of triggers) {
    const image = candidate.querySelector<HTMLImageElement>("img");
    const src = image?.currentSrc || image?.getAttribute("src") || "";
    if (!image || !src) continue;
    if (candidate === trigger) index = items.length;
    items.push({
      src,
      alt: image.alt,
      title: image.title || undefined,
      width: numericAttribute(image, "width"),
      height: numericAttribute(image, "height"),
    });
  }
  return items.length > 0 ? { items, index } : { items: [fallback], index: 0 };
}

/** A presentation-local Codex image gallery. Each trigger owns its dialog while the
 * gallery is discovered from the nearest exact message body at open time, so streamed
 * content is current and nested delegated messages never leak into their parent. */
export function ImagePreviewGallery({ item, titleFallback, trigger }: Props) {
  const t = useT();
  const [zoomed, setZoomed] = useState(false);
  const [gallery, setGallery] = useState<GalleryState | null>(null);

  const setGalleryIndex = useCallback((index: number) => {
    setGallery((current) =>
      current
        ? { ...current, index: Math.max(0, Math.min(index, current.items.length - 1)) }
        : null,
    );
  }, []);

  useEffect(() => {
    if (!zoomed || !gallery) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "ArrowLeft" && gallery.index > 0) {
        event.preventDefault();
        setGalleryIndex(gallery.index - 1);
      } else if (event.key === "ArrowRight" && gallery.index < gallery.items.length - 1) {
        event.preventDefault();
        setGalleryIndex(gallery.index + 1);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [gallery, setGalleryIndex, zoomed]);

  const active = gallery?.items[gallery.index] ?? item;
  const hasGallery = (gallery?.items.length ?? 0) > 1;

  return (
    <LightboxDialog
      open={zoomed}
      onOpenChange={(open) => {
        setZoomed(open);
        if (!open) setGallery(null);
      }}
      title={active.alt || titleFallback}
      closeOnContentClick
      className="p-2"
      trigger={trigger({
        "data-message-image-preview-trigger": "true",
        onClick: (event) => setGallery(collectGallery(event.currentTarget, item)),
      })}
    >
      <div className="relative">
        <img
          src={active.src}
          alt={active.alt}
          title={active.title}
          width={active.width}
          height={active.height}
          className="max-h-[86vh] max-w-full rounded-lg object-contain media-edge"
        />
        {hasGallery && (
          <>
            <IconButton
              icon="chevron-left"
              size="md"
              title={t("message.image.previous")}
              disabled={gallery!.index === 0}
              onClick={(event) => {
                event.stopPropagation();
                setGalleryIndex(gallery!.index - 1);
              }}
              className="absolute top-1/2 left-2 -translate-y-1/2 bg-media-scrim text-on-media hover:bg-media-scrim"
            />
            <IconButton
              icon="chevron-right"
              size="md"
              title={t("message.image.next")}
              disabled={gallery!.index === gallery!.items.length - 1}
              onClick={(event) => {
                event.stopPropagation();
                setGalleryIndex(gallery!.index + 1);
              }}
              className="absolute top-1/2 right-2 -translate-y-1/2 bg-media-scrim text-on-media hover:bg-media-scrim"
            />
          </>
        )}
      </div>
    </LightboxDialog>
  );
}
