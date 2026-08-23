import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
  type ClipboardEvent,
  type FormEvent,
  type KeyboardEvent,
  type ReactNode,
} from "react";

import type { ContentBlock, Recipe, RunRef } from "@lyra/runtime-contract";

import { useLocalization, type Translate } from "../localization/Localization";
import { RecipeSuggestions } from "./RecipeSuggestions";
import { expandRecipeInvocation, slashRecipeQuery } from "./recipeInvocation";

const maxAttachments = 6;
const maxImageBytes = 10 * 1024 * 1024;
const maxTextBytes = 1024 * 1024;

export interface ComposerAttachment {
  id: string;
  name: string;
  kind: "image" | "text";
  mime: string;
  data: string;
  bytes: number;
}

export interface ComposerDraft {
  text: string;
  attachments: ComposerAttachment[];
  history: string[];
}

interface ComposerProps {
  sessionId: string;
  draft: ComposerDraft;
  activeRun?: RunRef;
  recipes: Recipe[];
  pending: boolean;
  error?: string;
  attachmentPolicy: "text-only" | "multimodal";
  children?: ReactNode;
  onChange(update: (draft: ComposerDraft) => ComposerDraft): void;
  onSend(input: ContentBlock[], idempotencyKey: string): Promise<void>;
  onStop(): Promise<void>;
}

export const emptyComposerDraft: ComposerDraft = {
  text: "",
  attachments: [],
  history: [],
};

export function Composer(props: ComposerProps) {
  const { t } = useLocalization();
  const textarea = useRef<HTMLTextAreaElement>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const intent = useRef<{ fingerprint: string; key: string } | undefined>(
    undefined,
  );
  const [attachmentError, setAttachmentError] = useState<string>();
  const [historyIndex, setHistoryIndex] = useState(-1);
  const [recipeIndex, setRecipeIndex] = useState(0);
  const [dismissedRecipeText, setDismissedRecipeText] = useState<string>();
  const waiting = props.activeRun?.status === "waiting";
  const running = props.activeRun?.status === "running";
  const recipeQuery = slashRecipeQuery(props.draft.text);
  const recipeSuggestions = useMemo(
    () =>
      recipeQuery === undefined
        ? []
        : props.recipes
            .filter((recipe) =>
              recipe.name.toLowerCase().startsWith(recipeQuery),
            )
            .slice(0, 8),
    [props.recipes, recipeQuery],
  );
  const recipesOpen =
    recipeSuggestions.length > 0 && dismissedRecipeText !== props.draft.text;
  const activeRecipeIndex = Math.min(
    recipeIndex,
    Math.max(recipeSuggestions.length - 1, 0),
  );
  const rawInput = inputBlocks(props.draft);
  const sendInput = inputBlocks({
    ...props.draft,
    text: expandRecipeInvocation(props.draft.text, props.recipes),
  });
  const imageBlocked =
    props.attachmentPolicy === "text-only" &&
    props.draft.attachments.some((attachment) => attachment.kind === "image");
  const canSend =
    sendInput.length > 0 && !props.pending && !waiting && !imageBlocked;

  useEffect(() => setRecipeIndex(0), [recipeQuery, recipeSuggestions.length]);

  const resizeComposer = useCallback(() => {
    const element = textarea.current;
    if (element === null) return;
    element.style.height = "0px";
    element.style.height = `${Math.min(Math.max(element.scrollHeight, 48), 180)}px`;
  }, []);
  useLayoutEffect(() => {
    resizeComposer();
  }, [props.draft.text, resizeComposer]);
  useEffect(() => {
    const element = textarea.current;
    const container = element?.parentElement ?? null;
    if (
      element === null ||
      element === undefined ||
      container === null ||
      typeof ResizeObserver === "undefined"
    ) {
      return;
    }
    const observer = new ResizeObserver(resizeComposer);
    observer.observe(container);
    return () => observer.disconnect();
  }, [resizeComposer]);

  const update = (patch: Partial<ComposerDraft>) => {
    props.onChange((current) => ({ ...current, ...patch }));
  };

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!canSend) return;
    const rawFingerprint = JSON.stringify(rawInput);
    const fingerprint = JSON.stringify(sendInput);
    const sendIntent =
      intent.current?.fingerprint === fingerprint
        ? intent.current
        : { fingerprint, key: crypto.randomUUID() };
    intent.current = sendIntent;
    const submittedText = props.draft.text.trim();
    try {
      await props.onSend(sendInput, sendIntent.key);
      props.onChange((current) => {
        const unchanged =
          JSON.stringify(inputBlocks(current)) === rawFingerprint;
        const history =
          submittedText === ""
            ? current.history
            : [
                submittedText,
                ...current.history.filter((value) => value !== submittedText),
              ].slice(0, 50);
        return {
          ...current,
          ...(unchanged ? { text: "", attachments: [] } : {}),
          history,
        };
      });
      intent.current = undefined;
      setHistoryIndex(-1);
      setAttachmentError(undefined);
    } catch {
      // The owner presents the error and the exact SendIntent remains retryable.
    }
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.nativeEvent.isComposing || event.nativeEvent.keyCode === 229)
      return;
    if (recipesOpen) {
      if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault();
        const direction = event.key === "ArrowDown" ? 1 : -1;
        setRecipeIndex(
          (current) =>
            (current + direction + recipeSuggestions.length) %
            recipeSuggestions.length,
        );
        return;
      }
      if (event.key === "Escape") {
        event.preventDefault();
        setDismissedRecipeText(props.draft.text);
        return;
      }
      if (event.key === "Enter" || event.key === "Tab") {
        event.preventDefault();
        chooseRecipe(
          recipeSuggestions[activeRecipeIndex] ?? recipeSuggestions[0],
        );
        return;
      }
    }
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      event.currentTarget.form?.requestSubmit();
      return;
    }
    if (
      event.key === "ArrowUp" &&
      props.draft.text === "" &&
      props.draft.attachments.length === 0 &&
      props.draft.history.length > 0
    ) {
      event.preventDefault();
      const next = Math.min(historyIndex + 1, props.draft.history.length - 1);
      setHistoryIndex(next);
      update({ text: props.draft.history[next] ?? "" });
      return;
    }
    if (event.key === "ArrowDown" && historyIndex >= 0) {
      event.preventDefault();
      const next = historyIndex - 1;
      setHistoryIndex(next);
      update({ text: next < 0 ? "" : (props.draft.history[next] ?? "") });
    }
  };

  function chooseRecipe(recipe: Recipe | undefined) {
    if (recipe === undefined) return;
    setHistoryIndex(-1);
    setRecipeIndex(0);
    setDismissedRecipeText(undefined);
    update({ text: `/${recipe.name} ` });
    requestAnimationFrame(() => textarea.current?.focus());
  }

  const attach = async (files: File[]) => {
    setAttachmentError(undefined);
    try {
      const remaining = maxAttachments - props.draft.attachments.length;
      if (remaining <= 0) {
        throw new Error(t("composer.removeAttachmentFirst"));
      }
      const additions = await Promise.all(
        files
          .slice(0, remaining)
          .map((file) => readAttachment(file, props.attachmentPolicy, t)),
      );
      props.onChange((current) => ({
        ...current,
        attachments: [...current.attachments, ...additions].slice(
          0,
          maxAttachments,
        ),
      }));
      if (files.length > remaining) {
        setAttachmentError(
          t("composer.attachmentLimit", { count: maxAttachments }),
        );
      }
    } catch (error) {
      setAttachmentError(messageOf(error, t("composer.attachmentReadFailed")));
    }
  };

  const chooseFiles = (event: ChangeEvent<HTMLInputElement>) => {
    const files = [...(event.currentTarget.files ?? [])];
    event.currentTarget.value = "";
    void attach(files);
  };

  const pasteFiles = (event: ClipboardEvent<HTMLTextAreaElement>) => {
    const files = [...event.clipboardData.files];
    if (files.length === 0) return;
    event.preventDefault();
    void attach(files);
  };

  return (
    <form className="run-composer" onSubmit={submit}>
      {recipesOpen ? (
        <RecipeSuggestions
          sessionId={props.sessionId}
          recipes={recipeSuggestions}
          activeIndex={activeRecipeIndex}
          onChoose={chooseRecipe}
        />
      ) : null}
      {props.draft.attachments.length > 0 ? (
        <div
          className="composer-attachments"
          aria-label={t("composer.attachments")}
        >
          {props.draft.attachments.map((attachment) => (
            <span key={attachment.id}>
              <b>
                {attachment.kind === "image"
                  ? t("composer.image")
                  : t("composer.file")}
              </b>
              <span>{attachment.name}</span>
              <button
                type="button"
                aria-label={t("composer.removeAttachment", {
                  name: attachment.name,
                })}
                onClick={() =>
                  props.onChange((current) => ({
                    ...current,
                    attachments: current.attachments.filter(
                      (value) => value.id !== attachment.id,
                    ),
                  }))
                }
              >
                ×
              </button>
            </span>
          ))}
        </div>
      ) : null}
      <label className="sr-only" htmlFor={`composer-${props.sessionId}`}>
        {t("composer.messageLyra")}
      </label>
      <textarea
        ref={textarea}
        id={`composer-${props.sessionId}`}
        value={props.draft.text}
        rows={1}
        maxLength={24_000}
        placeholder={
          waiting
            ? t("composer.waitingPlaceholder")
            : running
              ? t("composer.runningPlaceholder")
              : t("composer.readyPlaceholder")
        }
        onChange={(event) => {
          setHistoryIndex(-1);
          setDismissedRecipeText(undefined);
          update({ text: event.currentTarget.value });
        }}
        onKeyDown={handleKeyDown}
        onPaste={pasteFiles}
        role="combobox"
        aria-autocomplete="list"
        aria-expanded={recipesOpen}
        aria-controls={
          recipesOpen ? `recipe-options-${props.sessionId}` : undefined
        }
        aria-activedescendant={
          recipesOpen
            ? `recipe-option-${props.sessionId}-${activeRecipeIndex}`
            : undefined
        }
      />
      <footer>
        <div className="composer-tools">
          <input
            ref={fileInput}
            className="sr-only"
            type="file"
            multiple
            accept={`${props.attachmentPolicy === "multimodal" ? "image/*," : ""}.txt,.md,.go,.ts,.tsx,.js,.jsx,.json,.yaml,.yml,.toml,.css,.html,.sh,.py,.rs`}
            onChange={chooseFiles}
          />
          <button
            className="composer-tool"
            type="button"
            disabled={props.pending}
            onClick={() => fileInput.current?.click()}
          >
            <span aria-hidden="true">＋</span>
            {t("composer.attach")}
          </button>
          {props.children}
          <span className="composer-hint">{t("composer.keyboardHint")}</span>
        </div>
        <div className="composer-actions">
          {running || waiting ? (
            <button
              className="stop-action"
              type="button"
              disabled={props.pending}
              onClick={() => void props.onStop().catch(() => undefined)}
            >
              <span aria-hidden="true" />
              {t("composer.stop")}
            </button>
          ) : null}
          <button className="send-action" type="submit" disabled={!canSend}>
            {props.pending
              ? t("composer.sending")
              : running
                ? t("composer.steer")
                : t("composer.send")}
            <span aria-hidden="true">↑</span>
          </button>
        </div>
      </footer>
      {attachmentError ? (
        <p className="composer-error" role="alert">
          {attachmentError}
        </p>
      ) : null}
      {imageBlocked ? (
        <p className="composer-error" role="alert">
          {t("composer.imagesUnsupported")}
        </p>
      ) : null}
      {props.error ? (
        <p className="composer-error" role="alert">
          {props.error}
        </p>
      ) : null}
    </form>
  );
}

function inputBlocks(draft: ComposerDraft): ContentBlock[] {
  const blocks: ContentBlock[] = [];
  const text = draft.text.trim();
  if (text !== "") blocks.push({ type: "text", text });
  for (const attachment of draft.attachments) {
    if (attachment.kind === "image") {
      blocks.push({
        type: "image",
        mime: attachment.mime,
        data: attachment.data,
      });
    } else {
      blocks.push({
        type: "text",
        text: `Attached file: ${attachment.name}\n\n${attachment.data}`,
      });
    }
  }
  return blocks;
}

async function readAttachment(
  file: File,
  policy: "text-only" | "multimodal",
  t: Translate,
): Promise<ComposerAttachment> {
  if (file.type.startsWith("image/")) {
    if (policy !== "multimodal") {
      throw new Error(t("composer.chooseImageModel"));
    }
    if (file.size > maxImageBytes) {
      throw new Error(t("composer.imageTooLarge", { name: file.name }));
    }
    return {
      id: crypto.randomUUID(),
      name: file.name,
      kind: "image",
      mime: file.type,
      data: bytesToBase64(new Uint8Array(await file.arrayBuffer())),
      bytes: file.size,
    };
  }
  if (file.size > maxTextBytes) {
    throw new Error(t("composer.fileTooLarge", { name: file.name }));
  }
  return {
    id: crypto.randomUUID(),
    name: file.name,
    kind: "text",
    mime: file.type || "text/plain",
    data: await file.text(),
    bytes: file.size,
  };
}

function bytesToBase64(bytes: Uint8Array) {
  let binary = "";
  for (let index = 0; index < bytes.length; index += 32_768) {
    binary += String.fromCharCode(...bytes.subarray(index, index + 32_768));
  }
  return window.btoa(binary);
}

function messageOf(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}
