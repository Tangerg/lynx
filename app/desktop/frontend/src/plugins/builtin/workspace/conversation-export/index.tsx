// Built-in plugin: export the current conversation.
//
// Two command-palette entries — "Export conversation as Markdown" and
// "... as JSON". No new UI surface; lives on the Cmd+K palette so it
// stays out of the way until needed.
//
// Source of truth: when the runtime advertises features.sessionExport,
// export goes through `sessions.export` — the SERVER-authoritative dump
// (full run structure + raw Items) delivered over the transport file
// channel (a short-lived download URL; content never rides the JSON-RPC
// envelope, API.md §7.2). The local view-state render below is the
// FALLBACK for runtimes without the feature — it's lossy by design
// (only what the fold projected into messages).
//
// Cherry Studio supports nine export targets (Markdown, DOCX, Notion,
// Obsidian, etc.); we ship the two formats that cover ~90% of "save
// this somewhere else" workflows: human-readable Markdown for paste-
// into-docs, and structured JSON for replay / re-import / archive.

import type { Message } from "@/protocol/run/viewState";
import { asSessionId } from "@/rpc";
import { definePlugin } from "@/plugins/sdk";
import { getConfig } from "@/plugins/sdk/config";
import { getContainer } from "@/main/container";
import { RUNTIME_BASE } from "@/main/config";
import { getCurrentSessionView } from "@/state/agentStore";
import { useRuntimeStore } from "@/state/runtimeStore";
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

// The export URL is transport-relative on HTTP ("/v2/files/..."-style local
// gated path) but may be absolute (file:// on InProcess/IPC) — resolve
// against the live api.baseUrl only when it's relative.
function resolveExportUrl(url: string): string {
  if (/^[a-z][a-z0-9+.-]*:/i.test(url)) return url; // already absolute (http/file/…)
  const base = (getConfig<string>("api.baseUrl") ?? RUNTIME_BASE).replace(/\/$/, "");
  return url.startsWith("/") ? base + url : `${base}/${url}`;
}

/** Server-authoritative export. Returns false when the runtime doesn't
 *  advertise the feature or the call fails — callers fall back to the
 *  local render so the command never dead-ends. */
async function exportServer(format: "md" | "json"): Promise<boolean> {
  const sid = useSessionStore.getState().activeSessionId;
  if (!sid) return false;
  if (useRuntimeStore.getState().capabilities?.features.sessionExport !== true) return false;
  try {
    const { url } = await getContainer().client().sessions.export(asSessionId(sid), format);
    const a = document.createElement("a");
    a.href = resolveExportUrl(url);
    a.download = ""; // filename comes from the server's Content-Disposition
    document.body.append(a);
    a.click();
    a.remove();
    return true;
  } catch (err) {
    console.warn("[export] sessions.export failed — falling back to local render:", err);
    return false;
  }
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
      run: async () => {
        if (!(await exportServer("md"))) exportMarkdown();
      },
    });
    host.commands.register({
      id: "chat.export.json",
      label: "Export conversation as JSON",
      icon: "code",
      group: "Chat",
      keywords: ["save", "download", "export", "archive"],
      run: async () => {
        if (!(await exportServer("json"))) exportJson();
      },
    });
  },
});
