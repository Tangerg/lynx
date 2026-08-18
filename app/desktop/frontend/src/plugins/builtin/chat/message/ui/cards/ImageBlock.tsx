// Inlined user-image block — renders a userMessage's image attachment as a
// rounded thumbnail; click zooms it full-size in a dialog lightbox. The wire form is mime + raw base64
// (MULTIMODAL_IMAGE_INPUT, API.md §4.3); the data URL is rebuilt here for <img>.

import { useMemo } from "react";
import { Pressable } from "@/ui";
import { imageSizeFromBase64 } from "@/lib/imageHeader";
import { useT } from "@/lib/i18n";
import { ImagePreviewGallery } from "../ImagePreviewGallery";

export function ImageBlock({ mime, data }: { mime: string; data: string }) {
  const t = useT();
  const src = `data:${mime};base64,${data}`;
  // The transcript has to know how tall this is before it decodes. Undimensioned, the
  // row measured 0 -> 0 -> 256px across the frames after mount, and everything below a
  // message is what moves. `imageSizeFromBase64` reads the header rather than the image,
  // so the ratio is available on the first render; null means an unreadable header, and
  // then the browser decides as it did before.
  const size = useMemo(() => imageSizeFromBase64(data), [data]);
  return (
    <ImagePreviewGallery
      item={{ src, alt: "", width: size?.width, height: size?.height }}
      titleFallback={t("message.image.view")}
      trigger={(previewProps) => (
        <Pressable
          type="button"
          aria-label={t("message.image.view")}
          {...previewProps}
          className="block cursor-zoom-in overflow-hidden rounded-md border-0 bg-transparent p-0"
        >
          <img
            src={src}
            alt=""
            width={size?.width}
            height={size?.height}
            className="max-h-64 max-w-full rounded-md object-contain media-edge"
          />
        </Pressable>
      )}
    />
  );
}
