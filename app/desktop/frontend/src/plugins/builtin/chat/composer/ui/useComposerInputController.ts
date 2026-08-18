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
import {
  composerCompositionKeyIntent,
  composerKeyBindingKey,
  composerPasteIntent,
} from "../application/composerInputEvents";
import { runtimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";

interface Args {
  value: string;
  onChange: (value: string) => void;
  onClear: () => void;
  onSend: (input: UserInput) => boolean;
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
  // Some Chinese IMEs commit raw Latin text with compositionend and then emit
  // an unmarked Enter from the same physical action. Keep that exact lifecycle
  // fact until the next key decision; keyup/focus/pointer boundaries retire it
  // deterministically when composition ended by another interaction.
  const compositionCommitPendingRef = useRef(false);
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
      submitComposer({
        value,
        clear: onClear,
        sendInput: onSend,
        images,
        pastes,
        recordHistory,
        canSend: runtimeCommandsAvailable,
      }),
    [images, onClear, onSend, pastes, recordHistory, value],
  );

  const handleChange = (event: ChangeEvent<HTMLTextAreaElement>): void => {
    const target = event.target;
    // Some browsers drop compositionend; recover a stuck flag when a plain
    // input arrives with no active native composition.
    const nativeComposing = (event.nativeEvent as { isComposing?: boolean }).isComposing === true;
    if (composingRef.current && !nativeComposing) {
      composingRef.current = false;
      compositionCommitPendingRef.current = true;
    }
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
    compositionCommitPendingRef.current = false;
  };

  const handleCompositionEnd = (event: CompositionEvent<HTMLTextAreaElement>): void => {
    composingRef.current = false;
    compositionCommitPendingRef.current = true;
    // Composition committed: sync the final value (event ordering vs. the last
    // input event varies by browser) and broadcast the caret once so the
    // mention/slash lookup runs against real text.
    const target = event.currentTarget;
    onChange(target.value);
    setCaret(target.selectionStart ?? target.value.length);
  };

  const handlePaste = (event: ClipboardEvent<HTMLTextAreaElement>): void => {
    compositionCommitPendingRef.current = false;
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
    compositionCommitPendingRef.current = false;
    if (files.length === 0 || !acceptsImages) return;
    onAddImages(files);
  };

  const clearCompositionCommit = (): void => {
    compositionCommitPendingRef.current = false;
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>): void => {
    // Consume the pending commit at the first key decision. Active/native/229
    // composition keys remain browser-owned; the otherwise-unmarked commit
    // Enter is also prevented so it neither sends nor inserts a stray newline.
    const compositionIntent = composerCompositionKeyIntent(
      event.nativeEvent,
      composingRef.current,
      compositionCommitPendingRef.current,
    );
    compositionCommitPendingRef.current = false;
    if (compositionIntent !== null) {
      if (compositionIntent === "committed-enter") event.preventDefault();
      return;
    }
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
    clearCompositionCommit,
    handleCompositionStart,
    handleCompositionEnd,
    handleDrop,
    handleKeyDown,
    handleKeyUp: clearCompositionCommit,
    handlePaste,
    handleSelect,
  };
}
