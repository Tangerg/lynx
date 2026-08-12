import type {
  ChangeEvent,
  ClipboardEvent,
  CompositionEvent,
  KeyboardEvent,
  SyntheticEvent,
} from "react";
import { useCallback, useEffect, useRef, useState } from "react";
import type { ComposerImage, PastedText } from "@/plugins/builtin/chat/composer/public/attachments";
import type { UserInput } from "@/plugins/builtin/chat/composer/public/input";
import { imageFiles } from "@/plugins/builtin/chat/composer/public/input";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { useFileMentions } from "@/plugins/builtin/chat/composer/public/fileMentions";
import { useIsCurrentRootRunning } from "@/plugins/builtin/agent/public/run";
import { COMPOSER_KEY_BINDING, lookupExtensionByKey } from "@/plugins/sdk";
import { submitComposer } from "@/plugins/builtin/chat/composer/public/submit";
import { setComposerFocusTarget } from "../application/focus";
import { useT } from "@/lib/i18n";
import { composerKeyBindingKey, composerPasteIntent } from "../application/composerInputEvents";

interface Args {
  value: string;
  onChange: (value: string) => void;
  onClear: () => void;
  onSend: (input: UserInput) => void;
  images: ComposerImage[];
  pastes: PastedText[];
  recordHistory: (text: string) => void;
  onAddImages: (files: File[]) => void;
  onAddPaste: (text: string) => void;
  acceptsImages: boolean;
}

export function useComposerInputController({
  value,
  onChange,
  onClear,
  onSend,
  images,
  pastes,
  recordHistory,
  onAddImages,
  onAddPaste,
  acceptsImages,
}: Args) {
  const t = useT();
  const inputRef = useRef<HTMLTextAreaElement>(null);
  // The controller owns this element, so it is the one that publishes it as the
  // context's focus target — see application/focus.ts for why that is a
  // capability and not a DOM query.
  useEffect(() => {
    setComposerFocusTarget(inputRef.current);
    return () => setComposerFocusTarget(null);
  }, []);
  const workspace = useActiveSessionWorkspace();
  const cwd = workspace.status === "ready" ? workspace.cwd : undefined;
  const [caret, setCaret] = useState(0);
  // IME composition guard (CJK-first). While a syllable is still being composed
  // the textarea fires intermediate change/select events; broadcasting the caret
  // then would drive the @-mention / slash lookup off half-composed text. We keep
  // the controlled value in sync throughout (React would otherwise snap the
  // textarea back to a stale value mid-composition) but defer the caret broadcast
  // until composition commits (compositionend).
  const composingRef = useRef(false);
  const applyMention = useCallback(
    (text: string, next: number) => {
      onChange(text);
      requestAnimationFrame(() => {
        const textarea = inputRef.current;
        if (textarea) {
          textarea.focus();
          textarea.setSelectionRange(next, next);
        }
        setCaret(next);
      });
    },
    [onChange],
  );
  const mentions = useFileMentions({ value, caret, cwd, apply: applyMention });
  const running = useIsCurrentRootRunning();
  const placeholder = running ? t("composer.placeholder.steer") : t("composer.placeholder");
  const submit = useCallback(
    () =>
      submitComposer({ value, clear: onClear, sendInput: onSend, images, pastes, recordHistory }),
    [images, onClear, onSend, pastes, recordHistory, value],
  );

  useEffect(() => {
    const textarea = inputRef.current;
    if (!textarea) return;
    textarea.style.height = "auto";
    textarea.style.height = `${Math.min(textarea.scrollHeight, 160)}px`;
  }, [value]);

  const handleChange = (event: ChangeEvent<HTMLTextAreaElement>): void => {
    const target = event.target;
    // Some browsers drop compositionend; recover a stuck flag when a plain
    // input arrives with no active native composition.
    const nativeComposing = (event.nativeEvent as { isComposing?: boolean }).isComposing === true;
    if (composingRef.current && !nativeComposing) composingRef.current = false;
    onChange(target.value);
    if (composingRef.current || nativeComposing) return;
    setCaret(target.selectionStart ?? target.value.length);
  };

  const handleSelect = (event: SyntheticEvent<HTMLTextAreaElement>): void => {
    if (composingRef.current) return;
    setCaret(event.currentTarget.selectionStart ?? 0);
  };

  const handleCompositionStart = (): void => {
    composingRef.current = true;
  };

  const handleCompositionEnd = (event: CompositionEvent<HTMLTextAreaElement>): void => {
    composingRef.current = false;
    // Composition committed: sync the final value (event ordering vs. the last
    // input event varies by browser) and broadcast the caret once so the
    // mention/slash lookup runs against real text.
    const target = event.currentTarget;
    onChange(target.value);
    setCaret(target.selectionStart ?? target.value.length);
  };

  const handlePaste = (event: ClipboardEvent<HTMLTextAreaElement>): void => {
    const files = imageFiles(event.clipboardData?.files);
    const text = files.length > 0 ? "" : (event.clipboardData?.getData("text") ?? "");
    const intent = composerPasteIntent(files, text);
    switch (intent.kind) {
      case "images":
        event.preventDefault();
        if (acceptsImages) onAddImages(intent.files);
        break;
      case "large-text":
        event.preventDefault();
        onAddPaste(intent.text);
        break;
    }
  };

  const handleDrop = (files: File[]): void => {
    if (files.length === 0 || !acceptsImages) return;
    onAddImages(files);
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>): void => {
    // BOTH sources, and our own first. `isComposing` is the browser's opinion and
    // WKWebView — which is the engine this app actually ships on — does not set it
    // on the keydown that commits a candidate for every IME. So an Enter pressed
    // mid-pinyin arrived here with `isComposing: false` and sent a half-typed
    // syllable as a message.
    //
    // `composingRef` is driven by compositionstart/compositionend, which WebKit does
    // fire, and this hook has been keeping it accurate for the caret logic all along
    // — the Enter path simply never asked. Recovery for a dropped compositionend
    // already exists in `handleChange`, so a stuck flag self-heals on the next
    // character rather than swallowing Enter forever.
    if (composingRef.current || event.nativeEvent.isComposing) return;
    if (mentions.handleKeyDown(event)) {
      event.preventDefault();
      return;
    }
    const binding = lookupExtensionByKey(
      COMPOSER_KEY_BINDING,
      composerKeyBindingKey(event.nativeEvent),
    );
    if (!binding) return;
    const handled = binding.handler({
      value,
      onChange,
      submit,
      event: event.nativeEvent,
    });
    if (handled) event.preventDefault();
  };

  return {
    inputRef,
    mentions,
    placeholder,
    handleChange,
    handleCompositionStart,
    handleCompositionEnd,
    handleDrop,
    handleKeyDown,
    handlePaste,
    handleSelect,
  };
}
