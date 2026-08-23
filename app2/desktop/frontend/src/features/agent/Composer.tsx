import {
  useLayoutEffect,
  useRef,
  useState,
  type ChangeEvent,
  type ClipboardEvent,
  type FormEvent,
  type KeyboardEvent,
} from "react";

import type { ContentBlock, RunRef } from "@lyra/runtime-contract";

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
  pending: boolean;
  error?: string;
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
  const textarea = useRef<HTMLTextAreaElement>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const intent = useRef<{ fingerprint: string; key: string } | undefined>(
    undefined,
  );
  const [attachmentError, setAttachmentError] = useState<string>();
  const [historyIndex, setHistoryIndex] = useState(-1);
  const waiting = props.activeRun?.status === "waiting";
  const running = props.activeRun?.status === "running";
  const input = inputBlocks(props.draft);
  const canSend = input.length > 0 && !props.pending && !waiting;

  useLayoutEffect(() => {
    const element = textarea.current;
    if (element === null) return;
    element.style.height = "0px";
    element.style.height = `${Math.min(Math.max(element.scrollHeight, 48), 180)}px`;
  }, [props.draft.text]);

  const update = (patch: Partial<ComposerDraft>) => {
    props.onChange((current) => ({ ...current, ...patch }));
  };

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!canSend) return;
    const fingerprint = JSON.stringify(input);
    const sendIntent =
      intent.current?.fingerprint === fingerprint
        ? intent.current
        : { fingerprint, key: crypto.randomUUID() };
    intent.current = sendIntent;
    const submittedText = props.draft.text.trim();
    try {
      await props.onSend(input, sendIntent.key);
      props.onChange((current) => {
        const unchanged = JSON.stringify(inputBlocks(current)) === fingerprint;
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
    if (
      event.key === "Enter" &&
      !event.shiftKey &&
      !event.nativeEvent.isComposing
    ) {
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

  const attach = async (files: File[]) => {
    setAttachmentError(undefined);
    try {
      const remaining = maxAttachments - props.draft.attachments.length;
      if (remaining <= 0) throw new Error("Remove an attachment before adding another.");
      const additions = await Promise.all(files.slice(0, remaining).map(readAttachment));
      props.onChange((current) => ({
        ...current,
        attachments: [...current.attachments, ...additions].slice(
          0,
          maxAttachments,
        ),
      }));
      if (files.length > remaining) {
        setAttachmentError(`Only ${maxAttachments} attachments can be sent at once.`);
      }
    } catch (error) {
      setAttachmentError(messageOf(error));
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
      {props.draft.attachments.length > 0 ? (
        <div className="composer-attachments" aria-label="Attachments">
          {props.draft.attachments.map((attachment) => (
            <span key={attachment.id}>
              <b>{attachment.kind === "image" ? "Image" : "File"}</b>
              <span>{attachment.name}</span>
              <button
                type="button"
                aria-label={`Remove ${attachment.name}`}
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
        Message Lyra
      </label>
      <textarea
        ref={textarea}
        id={`composer-${props.sessionId}`}
        value={props.draft.text}
        rows={1}
        maxLength={24_000}
        placeholder={
          waiting
            ? "This run is waiting for your response…"
            : running
              ? "Add guidance while Lyra works…"
              : "Describe what you want to accomplish…"
        }
        onChange={(event) => {
          setHistoryIndex(-1);
          update({ text: event.currentTarget.value });
        }}
        onKeyDown={handleKeyDown}
        onPaste={pasteFiles}
      />
      <footer>
        <div className="composer-tools">
          <input
            ref={fileInput}
            className="sr-only"
            type="file"
            multiple
            accept="image/*,.txt,.md,.go,.ts,.tsx,.js,.jsx,.json,.yaml,.yml,.toml,.css,.html,.sh,.py,.rs"
            onChange={chooseFiles}
          />
          <button
            className="composer-tool"
            type="button"
            disabled={props.pending}
            onClick={() => fileInput.current?.click()}
          >
            <span aria-hidden="true">＋</span>
            Attach
          </button>
          <span className="composer-hint">Enter to send · Shift+Enter for line break</span>
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
              Stop
            </button>
          ) : null}
          <button className="send-action" type="submit" disabled={!canSend}>
            {props.pending ? "Sending…" : running ? "Steer" : "Send"}
            <span aria-hidden="true">↑</span>
          </button>
        </div>
      </footer>
      {attachmentError ? (
        <p className="composer-error" role="alert">{attachmentError}</p>
      ) : null}
      {props.error ? (
        <p className="composer-error" role="alert">{props.error}</p>
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
      blocks.push({ type: "image", mime: attachment.mime, data: attachment.data });
    } else {
      blocks.push({
        type: "text",
        text: `Attached file: ${attachment.name}\n\n${attachment.data}`,
      });
    }
  }
  return blocks;
}

async function readAttachment(file: File): Promise<ComposerAttachment> {
  if (file.type.startsWith("image/")) {
    if (file.size > maxImageBytes) {
      throw new Error(`${file.name} is larger than 10 MB.`);
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
    throw new Error(`${file.name} is larger than 1 MB.`);
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

function messageOf(error: unknown) {
  return error instanceof Error ? error.message : "The attachment could not be read.";
}
