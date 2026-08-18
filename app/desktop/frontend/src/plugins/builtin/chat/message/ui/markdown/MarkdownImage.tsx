import { useState } from "react";
import { useT } from "@/lib/i18n";
import { Icon, LightboxDialog, Pressable } from "@/ui";

const INLINE_IMAGE = /^data:image\/(?:avif|gif|jpeg|jpg|png|svg\+xml|webp)(?:;[^,]*)?,/i;

export function isInlineMarkdownImage(src: string): boolean {
  return INLINE_IMAGE.test(src);
}

interface Props {
  src?: string;
  alt?: string;
  title?: string;
}

/** Restricted Desktop media renderer. Model-authored remote URLs never become
 *  image sources (and therefore cannot act as tracking pixels); explicitly
 *  inlined image data remains a first-class, zoomable artifact. */
export function MarkdownImage({ src = "", alt = "", title }: Props) {
  const t = useT();
  const [zoomed, setZoomed] = useState(false);
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
    <LightboxDialog
      open={zoomed}
      onOpenChange={setZoomed}
      title={previewLabel}
      closeOnContentClick
      className="p-2"
      trigger={
        <Pressable
          type="button"
          aria-label={previewLabel}
          className="my-3 inline-block cursor-zoom-in border-0 bg-transparent p-0 align-top"
        >
          <img
            src={src}
            alt={alt}
            title={title}
            loading="lazy"
            onError={() => setFailedSource(src)}
            className="block max-h-50 max-w-[min(100%,44rem)] rounded-md object-contain shadow-md media-edge"
          />
        </Pressable>
      }
    >
      <img
        src={src}
        alt={alt}
        title={title}
        className="max-h-[86vh] max-w-full rounded-lg object-contain media-edge"
      />
    </LightboxDialog>
  );
}
