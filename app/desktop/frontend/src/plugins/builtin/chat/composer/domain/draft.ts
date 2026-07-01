export interface ComposerDraftImage {
  mime: string;
  data: string;
  name?: string;
}

export interface ComposerDraftInput {
  text: string;
  images?: ComposerDraftImage[];
}
