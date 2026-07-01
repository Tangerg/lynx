export interface ComposerImageInput {
  mime: string;
  data: string;
}

export interface ComposerPasteInput {
  text: string;
}

export interface ComposerDraftInput {
  value: string;
  images: readonly ComposerImageInput[];
  pastes: readonly ComposerPasteInput[];
}

export interface SlashIntent {
  cmd: string;
  args: string;
}

export interface ComposerSendIntent {
  text: string;
  body: string;
  slash: SlashIntent | null;
  shouldSend: boolean;
  historyText: string | null;
}

export function createComposerSendIntent({
  value,
  images,
  pastes,
}: ComposerDraftInput): ComposerSendIntent {
  const text = value.trim();
  const body = [text, ...pastes.map((paste) => paste.text)].filter(Boolean).join("\n\n");
  const shouldSend = Boolean(text || images.length > 0 || pastes.length > 0);
  return {
    text,
    body,
    slash: text ? parseSlash(text) : null,
    shouldSend,
    historyText: text || null,
  };
}

export function parseSlash(text: string): SlashIntent | null {
  if (!text.startsWith("/")) return null;
  const space = text.search(/\s/);
  if (space === -1) return { cmd: text, args: "" };
  return { cmd: text.slice(0, space), args: text.slice(space + 1) };
}
