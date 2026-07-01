export interface ComposerDraftImage {
  mime: string;
  data: string;
  name?: string;
}

export interface ComposerDraftInput {
  text: string;
  images?: ComposerDraftImage[];
}

export interface ComposerImage extends ComposerDraftImage {
  id: string;
}

export interface PastedText {
  id: string;
  text: string;
  lines: number;
}
