import type { UserInput } from "@/plugins/builtin/chat/composer/public/input";
import { Composer, SlashSuggestions } from "@/plugins/builtin/chat/composer/public/ui";
import { useSelectedModel } from "@/plugins/builtin/chat/composer/public/selectedModel";
import {
  useAddComposerImageFiles,
  useAddComposerPaste,
  useComposerImages,
  useComposerPastes,
  useRemoveComposerImage,
  useRemoveComposerPaste,
} from "@/plugins/builtin/chat/composer/public/attachments";
import {
  useClearComposerDraft,
  useComposerText,
  useSetComposerText,
} from "@/plugins/builtin/chat/composer/public/draft";

// The composer surface — status strip, slash hints, input.
//
// Its own component so the draft text lives in the smallest component that needs
// it. Held one level up (in ChatStream, which also renders the transcript), every
// keystroke re-rendered the message list, and every streamed token re-rendered the
// composer. Neither has anything to say to the other: the transcript changes when
// the run produces something, the composer changes when the user types.
export function ComposerSurface({ onSend }: { onSend: (input: UserInput) => boolean }) {
  const value = useComposerText();
  const setValue = useSetComposerText();
  const clear = useClearComposerDraft();
  const images = useComposerImages();
  const removeImage = useRemoveComposerImage();
  const addImageFiles = useAddComposerImageFiles();
  const pastes = useComposerPastes();
  const removePaste = useRemoveComposerPaste();
  const addPaste = useAddComposerPaste();
  // Gate image staging on the next run's model accepting images — keeps the
  // paste/drop path consistent with the (disabled) toolbar attach button.
  const acceptsImages = useSelectedModel()?.acceptsInput("image") ?? false;

  return (
    <>
      <SlashSuggestions value={value} onPick={setValue} />
      <Composer
        value={value}
        onChange={setValue}
        onClear={clear}
        onSend={onSend}
        images={images}
        onRemoveImage={removeImage}
        onAddImages={addImageFiles}
        pastes={pastes}
        onRemovePaste={removePaste}
        onAddPaste={addPaste}
        acceptsImages={acceptsImages}
      />
    </>
  );
}
