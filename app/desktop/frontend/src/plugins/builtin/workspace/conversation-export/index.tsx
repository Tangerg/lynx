// Built-in plugin: export the current conversation.
//
// Two command-palette entries — "Export conversation as Markdown" and
// "... as JSON". No new UI surface; lives on the Cmd+K palette so it
// stays out of the way until needed.
//
// Cherry Studio supports nine export targets (Markdown, DOCX, Notion,
// Obsidian, etc.); we ship the two formats that cover ~90% of "save
// this somewhere else" workflows: human-readable Markdown for paste-
// into-docs, and structured JSON for replay / re-import / archive.

import type { Message } from "@/protocol/run/viewState";
import { definePlugin } from "@/plugins/sdk";
import { getCurrentSessionView } from "@/state/agentStore";
import { useSessionStore } from "@/state/sessionStore";
import { flattenMarkdown } from "@/lib/agent/messageContent";

function timestampForFilename(): string {
  // Filesystem-safe ISO slice (no `:` or `.`).
  return new Date().toISOString().replace(/[:.]/g, "-").slice(0, 19);
}

function downloadBlob(filename: string, content: string, mime: string): void {
  const blob = new Blob([content], { type: mime });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.append(a);
  a.click();
  a.remove();
  // 1s window so the browser actually picks the URL up before we
  // revoke it; revoking synchronously sometimes cancels the download
  // in WebKit.
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

function renderMessageMarkdown(msg: Message): string {
  const body = flattenMarkdown(msg.blocks).trim();
  // Skip messages whose blocks are entirely UI-only (e.g. a checkpoint
  // marker with no text) — they'd just leave an empty section.
  if (!body) return "";
  const headerName = msg.who || msg.role;
  return `## ${headerName} · ${msg.time}\n\n${body}\n`;
}

function exportMarkdown(): void {
  const view = getCurrentSessionView();
  const sid = useSessionStore.getState().activeSessionId;
  const sections: string[] = [
    `# Conversation \`${sid}\``,
    `*Exported ${new Date().toISOString()}*`,
    "",
  ];
  for (const msg of view.messages) {
    const rendered = renderMessageMarkdown(msg);
    if (rendered) sections.push(rendered);
  }
  downloadBlob(
    `lyra-${sid}-${timestampForFilename()}.md`,
    sections.join("\n"),
    "text/markdown;charset=utf-8",
  );
}

function exportJson(): void {
  const view = getCurrentSessionView();
  const sid = useSessionStore.getState().activeSessionId;
  // Versioned envelope so a future "Import conversation" can refuse
  // shapes it doesn't understand.
  const payload = {
    version: 1,
    sessionId: sid,
    exportedAt: new Date().toISOString(),
    messages: view.messages,
    plan: view.plan,
    timeline: view.timeline,
    toolCalls: view.toolCalls,
  };
  downloadBlob(
    `lyra-${sid}-${timestampForFilename()}.json`,
    JSON.stringify(payload, null, 2),
    "application/json;charset=utf-8",
  );
}

export default definePlugin({
  name: "lyra.builtin.conversation-export",
  version: "1.0.0",
  setup({ host }) {
    host.commands.register({
      id: "chat.export.markdown",
      label: "Export conversation as Markdown",
      icon: "filetext",
      group: "Chat",
      keywords: ["save", "download", "export"],
      run: exportMarkdown,
    });
    host.commands.register({
      id: "chat.export.json",
      label: "Export conversation as JSON",
      icon: "code",
      group: "Chat",
      keywords: ["save", "download", "export", "archive"],
      run: exportJson,
    });
  },
});
