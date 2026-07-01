import type { ChangeEvent, ClipboardEvent, KeyboardEvent, SyntheticEvent } from "react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ComposerImage } from "@/plugins/builtin/chat/composer/public/attachments";
import type { UserInput } from "@/plugins/builtin/chat/composer/public/input";
import { imageFiles } from "@/plugins/builtin/chat/composer/public/input";
import { isLargePaste } from "@/plugins/builtin/chat/composer/public/largePaste";
import { useActiveSessionCwd } from "@/plugins/builtin/agent/public/session";
import { useFileMentions } from "@/plugins/builtin/chat/composer/public/fileMentions";
import { useAgentRunning } from "@/state/agentStore";
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
  onAddImages,
  onAddPaste,
  acceptsImages,
}: Args) {
  const t = useT();
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const cwd = useActiveSessionCwd();
  const [caret, setCaret] = useState(0);
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
  const running = useAgentRunning();
  const placeholder = running ? t("composer.placeholder.steer") : basePlaceholder;
  const submit = useCallback(
    () => submitComposer({ value, clear: onClear, sendInput: onSend, images }),
    [images, onClear, onSend, value],
  );

  useEffect(() => {
    const textarea = inputRef.current;
    if (!textarea) return;
    textarea.style.height = "auto";
    textarea.style.height = `${Math.min(textarea.scrollHeight, 160)}px`;
  }, [value]);

  const handleChange = (event: ChangeEvent<HTMLTextAreaElement>): void => {
    onChange(event.target.value);
    setCaret(event.target.selectionStart ?? event.target.value.length);
  };

  const handleSelect = (event: SyntheticEvent<HTMLTextAreaElement>): void => {
    setCaret(event.currentTarget.selectionStart ?? 0);
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
    handleDrop,
    handleKeyDown,
    handlePaste,
    handleSelect,
  };
}
