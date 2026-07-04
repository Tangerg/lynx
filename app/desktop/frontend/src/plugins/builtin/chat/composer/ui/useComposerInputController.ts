import type {
  ChangeEvent,
  ClipboardEvent,
  CompositionEvent,
  KeyboardEvent,
  SyntheticEvent,
} from "react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ComposerImage, PastedText } from "@/plugins/builtin/chat/composer/public/attachments";
import type { UserInput } from "@/plugins/builtin/chat/composer/public/input";
import { imageFiles } from "@/plugins/builtin/chat/composer/public/input";
import { isLargePaste } from "@/plugins/builtin/chat/composer/public/largePaste";
import { useActiveSessionCwd } from "@/plugins/builtin/agent/public/session";
import { useFileMentions } from "@/plugins/builtin/chat/composer/public/fileMentions";
import { useIsAgentRunning } from "@/plugins/builtin/agent/public/run";
import { COMPOSER_KEY_BINDING, lookupExtensionByKey, pickComposerPlaceholder } from "@/plugins/sdk";
import { normalizeCombo } from "@/plugins/sdk/registry";
import { submitComposer } from "@/plugins/builtin/chat/composer/public/submit";
import { useT } from "@/lib/i18n";

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
  const cwd = useActiveSessionCwd();
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
  const basePlaceholder = useMemo(
    () => {
      const picked = pickComposerPlaceholder()?.text;
      return picked ? t(picked) : t("composer.placeholder.fallback");
    },
    // eslint-disable-next-line react/exhaustive-deps
    [],
  );
  const running = useIsAgentRunning();
  const placeholder = running ? t("composer.placeholder.steer") : basePlaceholder;
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
    if (files.length > 0) {
      event.preventDefault();
      if (acceptsImages) onAddImages(files);
      return;
    }
    const text = event.clipboardData?.getData("text") ?? "";
    if (isLargePaste(text)) {
      event.preventDefault();
      onAddPaste(text);
    }
  };

  const handleDrop = (files: File[]): void => {
    if (files.length === 0 || !acceptsImages) return;
    onAddImages(files);
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>): void => {
    if (event.nativeEvent.isComposing) return;
    if (mentions.handleKeyDown(event)) {
      event.preventDefault();
      return;
    }
    const parts: string[] = [];
    if (event.metaKey || event.ctrlKey) parts.push("mod");
    if (event.altKey) parts.push("alt");
    if (event.shiftKey) parts.push("shift");
    parts.push(event.key);
    const binding = lookupExtensionByKey(COMPOSER_KEY_BINDING, normalizeCombo(parts.join("+")));
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
