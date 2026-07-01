export interface UserTextInput {
  kind: "text";
  text: string;
}

export interface UserImageInput {
  kind: "image";
  mime: string;
  data: string;
}

export type UserInputPart = UserTextInput | UserImageInput;

export interface UserInput {
  parts: UserInputPart[];
}

/** One image attachment ready to inline. `data` is raw base64 (no data: prefix). */
export interface InputImage {
  mime: string;
  data: string;
}

/** Plain-text input — the common programmatic case. Empty text → empty input. */
export function textInput(text: string): UserInput {
  return text ? { parts: [{ kind: "text", text }] } : { parts: [] };
}

/** Compose text + inlined images into a user input intent. */
export function buildInput(text: string, images: InputImage[]): UserInput {
  const parts: UserInputPart[] = [];
  if (text) parts.push({ kind: "text", text });
  for (const img of images) parts.push({ kind: "image", mime: img.mime, data: img.data });
  return { parts };
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
  // Guard against an unrecognised MIME that produces a data URL without the
  // expected comma separator — indexOf returns -1, which would otherwise
  // cause slice(0) to pass the prefix through as base64 data.
  const comma = dataUrl.indexOf(",");
  const data = comma >= 0 ? dataUrl.slice(comma + 1) : dataUrl;
  return { mime: file.type, data, name: file.name };
}

/** Filter a FileList (clipboard / drop / file-picker) down to image/* files. */
export function imageFiles(list: FileList | null | undefined): File[] {
  return list ? Array.from(list).filter((f) => f.type.startsWith("image/")) : [];
}
