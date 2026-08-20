import { getContainer } from "@/main/container";

/** Hand one already-renderable inline image to the packaged Desktop owner. The
 * adapter deliberately exposes no path and no browser fallback. */
export function saveInlineImage(source: string): Promise<boolean> {
  return getContainer().desktop.saveImage(source);
}
