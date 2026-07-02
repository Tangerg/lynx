// Inlined user-image block — renders a userMessage's image attachment as a
// rounded thumbnail; click zooms it full-size in a dialog lightbox. The wire form is mime + raw base64
// (MULTIMODAL_IMAGE_INPUT, API.md §4.3); the data URL is rebuilt here for <img>.

import { useState } from "react";
import { LightboxDialog } from "@/components/common";
import { useT } from "@/lib/i18n";

export function ImageBlock({ mime, data }: { mime: string; data: string }) {
  const t = useT();
  const [zoomed, setZoomed] = useState(false);
  const src = `data:${mime};base64,${data}`;
  return (
    <LightboxDialog
      open={zoomed}
      onOpenChange={setZoomed}
      title={t("message.image.view")}
      closeOnContentClick
      className="p-2"
      trigger={
        <button
          type="button"
          aria-label={t("message.image.view")}
          className="my-1.5 block cursor-zoom-in overflow-hidden rounded-md border-0 bg-transparent p-0 outline-none focus-visible:shadow-[0_0_0_2px_var(--color-accent)]"
        >
          <img
            src={src}
            alt=""
            className="max-h-64 max-w-full rounded-md object-contain border-[0.5px] border-white/10 light:border-black/10"
          />
        </button>
      }
    >
      <img
        src={src}
        alt=""
        className="max-h-[86vh] max-w-full rounded-lg object-contain border-[0.5px] border-white/10 light:border-black/10"
      />
    </LightboxDialog>
  );
}
