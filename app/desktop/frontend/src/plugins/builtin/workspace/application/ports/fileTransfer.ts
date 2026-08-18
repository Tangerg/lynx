/**
 * Handing a file to the user, and taking one back.
 *
 * A port because the use case ("export this conversation as markdown") is ours
 * while the browser owns the mechanism (an anchor with a blob URL or a hidden
 * file input).
 */
export interface FileTransferPort {
  download(filename: string, content: string, mime: string): void;
  /** Resolves the chosen file's text, or null when the picker is cancelled. */
  pickText(accept: string): Promise<string | null>;
}
