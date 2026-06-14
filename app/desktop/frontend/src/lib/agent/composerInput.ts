// User input → wire ContentBlock[] builders. The composer's send unit is the
// full user-message input (text + inlined images), NOT a bare string: image
// blocks carry mime + base64 inline (MULTIMODAL_IMAGE_INPUT, API.md §4.3).
// `textInput` is the plain-text shortcut every programmatic sender (slash
// command / regenerate / resend) uses; `buildInput` composes the composer's
// text with its image attachments.
//
// Lives in lib/ (not components/) so UI surfaces import the `UserInput` type
// here without reaching into @/rpc directly (the components↛rpc layer rule).

import type { ContentBlock } from "@/rpc";

/** A user-message body on the wire — text and/or inlined image blocks. */
export type UserInput = ContentBlock[];

/** One image attachment ready to inline. `data` is raw base64 (no data: prefix). */
export interface InputImage {
  mime: string;
  data: string;
}

/** Plain-text input — the common programmatic case. Empty text → empty input. */
export function textInput(text: string): UserInput {
  return text ? [{ type: "text", text }] : [];
}

/** Compose text + inlined images into a user-message input: a leading text
 *  block (when non-empty) then one image block per attachment. */
export function buildInput(text: string, images: InputImage[]): UserInput {
  const blocks: UserInput = [];
  if (text) blocks.push({ type: "text", text });
  for (const img of images) blocks.push({ type: "image", mime: img.mime, data: img.data });
  return blocks;
}

/** Read an image File into the inline wire form (mime + raw base64, no "data:"
 *  prefix). Used by the composer's paste / drop / file-picker paths; the caller
 *  pre-filters to image/* files. */
export async function fileToInputImage(file: File): Promise<InputImage & { name: string }> {
  const dataUrl = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(reader.error ?? new Error("file read failed"));
    reader.readAsDataURL(file);
  });
  // Strip the leading "data:<mime>;base64," → raw base64 (the wire form).
  return { mime: file.type, data: dataUrl.slice(dataUrl.indexOf(",") + 1), name: file.name };
}

/** Filter a FileList (clipboard / drop / file-picker) down to image/* files. */
export function imageFiles(list: FileList | null | undefined): File[] {
  return list ? Array.from(list).filter((f) => f.type.startsWith("image/")) : [];
}
