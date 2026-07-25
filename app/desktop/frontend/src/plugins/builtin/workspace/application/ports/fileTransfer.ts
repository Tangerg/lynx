import { createSingletonPort } from "@/lib/ports/singletonPort";

/**
 * Handing a file to the user, and taking one back.
 *
 * A port because the use case ("export this conversation as markdown") is ours
 * while the mechanism (an anchor with a blob URL, a hidden file input) is the
 * browser's. Both used to sit in the same application module, which is how
 * "download it" and "this is how Chromium saves a file" became one thing.
 */
export interface FileTransferPort {
  download(filename: string, content: string, mime: string): void;
  /** Resolves the chosen file's text, or null when the picker is cancelled. */
  pickText(accept: string): Promise<string | null>;
}

const port = createSingletonPort<FileTransferPort>("File transfer port is not configured");

export const configureFileTransferPort = port.configure;
export const fileTransfer = port.get;
