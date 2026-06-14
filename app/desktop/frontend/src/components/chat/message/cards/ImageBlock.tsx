// Inlined user-image block — renders a userMessage's image attachment as a
// rounded thumbnail; click zooms it full-size in a Radix Dialog lightbox (same
// affordance as MermaidBlock). The wire form is mime + raw base64
// (MULTIMODAL_IMAGE_INPUT, API.md §4.3); the data URL is rebuilt here for <img>.

import * as Dialog from "@radix-ui/react-dialog";
import { useState } from "react";

export function ImageBlock({ mime, data }: { mime: string; data: string }) {
  const [zoomed, setZoomed] = useState(false);
  const src = `data:${mime};base64,${data}`;
  return (
    <Dialog.Root open={zoomed} onOpenChange={setZoomed}>
      <Dialog.Trigger asChild>
        <button
          type="button"
          aria-label="View attached image"
          className="my-1.5 block cursor-zoom-in overflow-hidden rounded-lg border-0 bg-transparent p-0 outline-none focus-visible:shadow-[0_0_0_2px_var(--color-accent)]"
        >
          <img src={src} alt="" className="max-h-64 max-w-full rounded-lg object-contain" />
        </button>
      </Dialog.Trigger>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-[200] cursor-zoom-out bg-black/60 backdrop-blur-[8px] light:bg-black/25" />
        <Dialog.Content
          onClick={() => setZoomed(false)}
          className="fixed inset-0 z-[201] m-auto h-fit w-fit max-h-[90vh] max-w-[min(1400px,95vw)] cursor-zoom-out overflow-auto rounded-xl border border-line-soft bg-surface p-2 shadow-lg outline-none data-[state=open]:animate-rise-in"
        >
          <Dialog.Title className="sr-only">Attached image</Dialog.Title>
          <img src={src} alt="" className="max-h-[86vh] max-w-full rounded-lg object-contain" />
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
