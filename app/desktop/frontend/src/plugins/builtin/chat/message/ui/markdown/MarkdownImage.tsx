import { useState } from "react";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { Icon, Pressable } from "@/ui";
import { ImagePreviewGallery } from "../ImagePreviewGallery";

const INLINE_IMAGE = /^data:image\/(?:avif|gif|jpeg|jpg|png|svg\+xml|webp)(?:;[^,]*)?,/i;

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

/** Restricted Desktop media renderer. Model-authored remote URLs never become
 *  image sources (and therefore cannot act as tracking pixels); explicitly
 *  inlined image data remains a first-class, zoomable artifact. */
export function MarkdownImage({ src = "", alt = "", title, allowWide = false }: Props) {
  const t = useT();
  const [failedSource, setFailedSource] = useState<string | null>(null);
  const unavailable = !isInlineMarkdownImage(src) || failedSource === src;

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
  return (
    <ImagePreviewGallery
      item={{ src, alt, title }}
      titleFallback={previewLabel}
      trigger={(previewProps) => (
        <Pressable
          type="button"
          aria-label={previewLabel}
          {...previewProps}
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
      )}
    />
  );
}
