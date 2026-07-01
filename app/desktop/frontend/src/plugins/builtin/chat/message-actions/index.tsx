// Built-in plugins: per-message action buttons in `message.actions`.
//
// Four icon-only buttons rendered below the message (copy + edit +
// regenerate + feedback). Each is its own plugin so a fork that doesn't
// want one can drop it without touching the others. Shared chrome /
// helpers live in _shared.ts.

import { useEffect, useState } from "react";
import { DropdownMenu, Icon, Tooltip } from "@/components/common";
import { writeToClipboard } from "@/lib/clipboard";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { definePlugin, useCurrentMessage } from "@/plugins/sdk";
import { editMessageInComposer, regenerateMessage } from "./application/messageActions";
import { messageFeedbackRating, submitMessageFeedback } from "./public/feedback";
import { messageCopyPayloads } from "./presentation/copyPayloads";
import { ACTION_BTN_BASE } from "./_shared";

function roleShape(role: string): string {
  return role === "user" ? "rounded-full" : "rounded-md";
}

//
// Default click writes Markdown (preserves headings / lists / fences
// as they were rendered). The submenu surfaces the two alternates:
// Plain text drops markup so it pastes flat into editors, Code only
// concatenates the fenced code blocks for users who just want the
// generated snippets. Code variant hides when the message has none.

function CopyButton() {
  const t = useT();
  const msg = useCurrentMessage();
  const copy = messageCopyPayloads(msg);
  if (!copy.canCopy) return null;

  return (
    <DropdownMenu.Root>
      <Tooltip label={t("msgActions.copy")}>
        <DropdownMenu.Trigger
          aria-label={t("msgActions.copy")}
          className={cn(ACTION_BTN_BASE, roleShape(msg.role))}
        >
          <Icon name="copy" size={13} />
        </DropdownMenu.Trigger>
      </Tooltip>
      <DropdownMenu.Content align="end" sideOffset={4} className="min-w-[160px]">
        <CopyItem
          label={t("msgActions.copyMarkdown")}
          hint={t("msgActions.copyMarkdownHint")}
          onSelect={() =>
            writeToClipboard(copy.markdown || copy.plain, {
              successLabel: t("msgActions.copiedMarkdown"),
            })
          }
        />
        <CopyItem
          label={t("msgActions.copyPlain")}
          hint={t("msgActions.copyPlainHint")}
          onSelect={() =>
            writeToClipboard(copy.plain, { successLabel: t("msgActions.copiedPlain") })
          }
        />
        {copy.code && (
          <CopyItem
            label={t("msgActions.copyCode")}
            hint={t("msgActions.copyCodeHint")}
            onSelect={() =>
              writeToClipboard(copy.code, { successLabel: t("msgActions.copiedCode") })
            }
          />
        )}
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  );
}

function CopyItem({
  label,
  hint,
  onSelect,
}: {
  label: string;
  hint: string;
  onSelect: () => void;
}) {
  return (
    <DropdownMenu.Item
      onClick={onSelect}
      className="flex flex-col gap-0.5 rounded-sm px-2.5 py-1.5 outline-none data-[highlighted]:bg-surface-2"
    >
      <span className="text-[12.5px] text-fg">{label}</span>
      <span className="text-[11px] text-fg-faint">{hint}</span>
    </DropdownMenu.Item>
  );
}

export const messageCopy = definePlugin({
  name: "lyra.builtin.message-copy",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("message.actions", {
      id: "copy",
      order: 0,
      component: CopyButton,
    });
  },
});

// ---- Edit (user messages only): load the text back into the composer
// so the user can tweak and re-send. Doesn't mutate the original message
// — sending creates a new user turn. ----

function EditButton() {
  const t = useT();
  const msg = useCurrentMessage();
  if (msg.role !== "user") return null;
  if (!messageCopyPayloads(msg).plain) return null;

  return (
    <Tooltip label={t("msgActions.edit")}>
      <button
        type="button"
        onClick={() => editMessageInComposer(msg)}
        aria-label={t("msgActions.edit")}
        className={cn(ACTION_BTN_BASE, roleShape(msg.role))}
      >
        <Icon name="edit" size={13} />
      </button>
    </Tooltip>
  );
}

export const messageEdit = definePlugin({
  name: "lyra.builtin.message-edit",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("message.actions", {
      id: "edit",
      order: 5,
      component: EditButton,
    });
  },
});

// ---- Regenerate (assistant messages only): replay the preceding user
// prompt via the shared regenerate message action.

function RegenerateButton() {
  const t = useT();
  const msg = useCurrentMessage();
  if (msg.role !== "assistant") return null;

  return (
    <Tooltip label={t("msgActions.regenerate")}>
      <button
        type="button"
        onClick={() => regenerateMessage(msg)}
        aria-label={t("msgActions.regenerate")}
        className={cn(ACTION_BTN_BASE, roleShape(msg.role))}
      >
        <Icon name="loop" size={13} />
      </button>
    </Tooltip>
  );
}

export const messageRegenerate = definePlugin({
  name: "lyra.builtin.message-regenerate",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("message.actions", {
      id: "regenerate",
      order: 10,
      component: RegenerateButton,
    });
  },
});

// ---- Feedback (assistant messages only): thumbs up / down wired to
// `feedback.create`. The wire is write-only (no read-back API), so the
// settled rating lives in a session-lifetime map — same scope as the
// approval "remember" decisions. Re-rating re-submits; the runtime
// treats each as a new event. ----

function FeedbackButtons() {
  const t = useT();
  const msg = useCurrentMessage();
  const [rated, setRated] = useState(() => messageFeedbackRating(msg.id));
  useEffect(() => {
    setRated(messageFeedbackRating(msg.id));
  }, [msg.id]);
  if (msg.role !== "assistant") return null;

  const rate = (rating: "positive" | "negative"): void => {
    if (rated === rating) return;
    setRated(rating);
    void submitMessageFeedback(msg, rating).catch(() => setRated(messageFeedbackRating(msg.id)));
  };

  return (
    <>
      <Tooltip label={t("msgActions.good")}>
        <button
          type="button"
          onClick={() => rate("positive")}
          aria-label={t("msgActions.good")}
          aria-pressed={rated === "positive"}
          className={cn(
            ACTION_BTN_BASE,
            roleShape(msg.role),
            rated === "positive" && "text-success",
          )}
        >
          <Icon name="thumbs-up" size={13} />
        </button>
      </Tooltip>
      <Tooltip label={t("msgActions.poor")}>
        <button
          type="button"
          onClick={() => rate("negative")}
          aria-label={t("msgActions.poor")}
          aria-pressed={rated === "negative"}
          className={cn(
            ACTION_BTN_BASE,
            roleShape(msg.role),
            rated === "negative" && "text-negative",
          )}
        >
          <Icon name="thumbs-down" size={13} />
        </button>
      </Tooltip>
    </>
  );
}

export const messageFeedback = definePlugin({
  name: "lyra.builtin.message-feedback",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("message.actions", {
      id: "feedback",
      order: 15,
      component: FeedbackButtons,
    });
  },
});
