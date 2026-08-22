// Copy action — a dropdown in `message.actions`. Default click writes Markdown
// (preserves headings / lists / fences as rendered). The submenu surfaces the
// alternate: Plain text drops markup so it pastes flat into editors.

import { DropdownMenu, Tooltip } from "@/ui";
import { writeToClipboard } from "@/lib/clipboard";
import { useT } from "@/lib/i18n";
import { contributeLayout, definePlugin, useCurrentMessage } from "@/plugins/sdk";
import { canCopyMessage } from "./application/messageActionAvailability";
import { messageCopyPayloads } from "./presentation/copyPayloads";
import { MessageActionButton } from "./MessageActionButton";

function CopyButton() {
  const t = useT();
  const msg = useCurrentMessage();
  const copy = messageCopyPayloads(msg);
  if (!canCopyMessage(copy)) return null;

  return (
    <DropdownMenu.Root>
      <Tooltip label={t("msgActions.copy")}>
        <DropdownMenu.Trigger
          render={
            <MessageActionButton icon="copy" role={msg.role} aria-label={t("msgActions.copy")} />
          }
        />
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
      <span className="text-ui-md text-fg">{label}</span>
      <span className="text-ui-sm text-fg-faint">{hint}</span>
    </DropdownMenu.Item>
  );
}

export const messageCopy = definePlugin({
  name: "lyra.builtin.message-copy",
  setup(ctx) {
    contributeLayout(ctx, "message.actions", { id: "copy", order: 0, component: CopyButton });
  },
});
