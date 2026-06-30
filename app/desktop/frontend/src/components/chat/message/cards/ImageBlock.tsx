// Inlined user-image block — renders a userMessage's image attachment as a
// rounded thumbnail; click zooms it full-size in a dialog lightbox. The wire form is mime + raw base64
// (MULTIMODAL_IMAGE_INPUT, API.md §4.3); the data URL is rebuilt here for <img>.

import { useState } from "react";
import { Dialog as BaseDialog } from "@/components/common";
import { useT } from "@/lib/i18n";

export function ImageBlock({ mime, data }: { mime: string; data: string }) {
  const t = useT();
  const [zoomed, setZoomed] = useState(false);
  const src = `data:${mime};base64,${data}`;
  return (
    <BaseDialog.Root open={zoomed} onOpenChange={setZoomed}>
      <BaseDialog.Trigger
        render={
          <button
            type="button"
            aria-label={t("message.image.view")}
            className="my-1.5 block cursor-zoom-in overflow-hidden rounded-md border-0 bg-transparent p-0 outline-none focus-visible:shadow-[0_0_0_2px_var(--color-accent)]"
          >
            <img src={src} alt="" className="max-h-64 max-w-full rounded-md object-contain" />
          </button>
        }
      />
      <BaseDialog.Portal>
        <BaseDialog.Backdrop className="fixed inset-0 z-[200] cursor-zoom-out bg-black/60 light:bg-black/25" />
        <BaseDialog.Popup
          onClick={() => setZoomed(false)}
          className="fixed inset-0 z-[201] m-auto h-fit w-fit max-h-[90vh] max-w-[min(1400px,95vw)] cursor-zoom-out overflow-auto rounded-lg border-0 bg-surface p-2 shadow-[var(--shadow-popover)] outline-none data-[open]:animate-rise-in"
        >
          <BaseDialog.Title className="sr-only">{t("message.image.view")}</BaseDialog.Title>
          <img src={src} alt="" className="max-h-[86vh] max-w-full rounded-lg object-contain" />
        </BaseDialog.Popup>
      </BaseDialog.Portal>
    </BaseDialog.Root>
  );
}
