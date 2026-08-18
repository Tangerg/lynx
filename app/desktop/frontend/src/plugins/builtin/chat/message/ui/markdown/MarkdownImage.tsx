import { useCallback, useEffect, useState } from "react";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { Icon, IconButton, LightboxDialog, Pressable } from "@/ui";

const INLINE_IMAGE = /^data:image\/(?:avif|gif|jpeg|jpg|png|svg\+xml|webp)(?:;[^,]*)?,/i;
const PREVIEW_TRIGGER_ATTR = "data-markdown-image-preview-trigger";

export function isInlineMarkdownImage(src: string): boolean {
  return INLINE_IMAGE.test(src);
}

interface Props {
  src?: string;
  alt?: string;
  title?: string;
  /** Media-only paragraphs can borrow the full reading measure. */
  allowWide?: boolean;
}

interface GalleryItem {
  src: string;
  alt: string;
}

interface GalleryState {
  items: GalleryItem[];
  index: number;
}

function collectGallery(trigger: HTMLButtonElement, fallback: GalleryItem): GalleryState {
  const root = trigger.closest(".md");
  if (!root) return { items: [fallback], index: 0 };
  const triggers = Array.from(
    root.querySelectorAll<HTMLButtonElement>(`button[${PREVIEW_TRIGGER_ATTR}="true"]`),
  );
  const items: GalleryItem[] = [];
  let index = 0;
  for (const candidate of triggers) {
    const image = candidate.querySelector<HTMLImageElement>("img");
    const imageSrc = image?.currentSrc || image?.getAttribute("src") || "";
    if (!imageSrc) continue;
    if (candidate === trigger) index = items.length;
    items.push({ src: imageSrc, alt: image?.alt ?? "" });
  }
  return items.length > 0 ? { items, index } : { items: [fallback], index: 0 };
}

/** Restricted Desktop media renderer. Model-authored remote URLs never become
 *  image sources (and therefore cannot act as tracking pixels); explicitly
 *  inlined image data remains a first-class, zoomable artifact. */
export function MarkdownImage({ src = "", alt = "", title, allowWide = false }: Props) {
  const t = useT();
  const [zoomed, setZoomed] = useState(false);
  const [gallery, setGallery] = useState<GalleryState | null>(null);
  const [failedSource, setFailedSource] = useState<string | null>(null);
  const unavailable = !isInlineMarkdownImage(src) || failedSource === src;

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

  if (unavailable) {
    return (
      <Pressable
        type="button"
        disabled
        aria-label={alt || t("message.image.unavailable")}
        title={title}
        className="my-3 inline-flex min-h-24 min-w-24 cursor-default items-center justify-center rounded-md border-0 bg-sunken p-0 text-fg-faint"
      >
        <Icon name="image" size="md" />
      </Pressable>
    );
  }

  const previewLabel = alt || t("message.image.preview");
  const active = gallery?.items[gallery.index] ?? { src, alt };
  const hasGallery = (gallery?.items.length ?? 0) > 1;
  return (
    <LightboxDialog
      open={zoomed}
      onOpenChange={(open) => {
        setZoomed(open);
        if (!open) setGallery(null);
      }}
      title={active.alt || previewLabel}
      closeOnContentClick
      className="p-2"
      trigger={
        <Pressable
          type="button"
          aria-label={previewLabel}
          {...{ [PREVIEW_TRIGGER_ATTR]: "true" }}
          onClick={(event) => setGallery(collectGallery(event.currentTarget, { src, alt }))}
          className={cn(
            "my-3 inline-block cursor-zoom-in border-0 bg-transparent p-0 align-top",
            allowWide ? "max-w-full" : "max-w-[min(100%,44rem)]",
          )}
        >
          <img
            src={src}
            alt={alt}
            title={title}
            loading="lazy"
            onError={() => setFailedSource(src)}
            className="block max-h-50 max-w-full rounded-md object-contain shadow-md media-edge"
          />
        </Pressable>
      }
    >
      <div className="relative">
        <img
          src={active.src}
          alt={active.alt}
          title={title}
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
