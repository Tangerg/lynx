// Built-in plugin: export / import the current conversation.
//
// Command-palette entries only — "Export conversation as Markdown",
// "... as JSON", and "Import conversation from JSON". No new UI
// surface; lives on the Cmd+K palette so it stays out of the way.
//
// Source of truth: when the runtime advertises features.sessionExport,
// export goes through `sessions.export` — the SERVER-authoritative dump
// returned INLINE (API.md §7.2: `markdown` transcript or a round-trippable
// `artifact` that `sessions.import` restores under its original id). The
// local view-state render below is the FALLBACK for runtimes without the
// feature — it's lossy by design (only what the fold projected into
// messages) and NOT importable.
//
// Cherry Studio supports nine export targets (Markdown, DOCX, Notion,
// Obsidian, etc.); we ship the two formats that cover ~90% of "save
// this somewhere else" workflows: human-readable Markdown for paste-
// into-docs, and structured JSON for replay / re-import / archive.

import type { Message } from "@/protocol/run/viewState";
import type { SessionArtifact } from "@/rpc";
import { toast } from "sonner";
import { z } from "zod";
import { notifyError } from "@/lib/notify";
import { t } from "@/lib/i18n";
import { asSessionId } from "@/rpc";
import { definePlugin } from "@/plugins/sdk";
import { getContainer } from "@/main/container";
import { getCurrentSessionView } from "@/state/agentStore";
import { serverFeature } from "@/state/runtimeStore";
import { useSessionStore } from "@/state/sessionStore";
import { invalidateSessions } from "@/lib/data/queries";
import { flattenMarkdown } from "@/lib/agent/messageContent";
import { rehydrateSessionView } from "@/lib/agent/rehydrateSession";

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

/** Server-authoritative export (inline payload). Returns false when the
 *  runtime doesn't advertise the feature or the call fails — callers fall
 *  back to the local render so the command never dead-ends. */
async function exportServer(format: "md" | "json"): Promise<boolean> {
  const sid = useSessionStore.getState().activeSessionId;
  if (!sid) return false;
  if (!serverFeature("sessionExport")) return false;
  try {
    const resp = await getContainer().client().sessions.export(asSessionId(sid), format);
    const stamp = timestampForFilename();
    if (resp.format === "md" && resp.markdown !== undefined) {
      downloadBlob(`lyra-${sid}-${stamp}.md`, resp.markdown, "text/markdown;charset=utf-8");
      return true;
    }
    if (resp.format === "json" && resp.artifact) {
      downloadBlob(
        `lyra-${sid}-${stamp}.json`,
        JSON.stringify(resp.artifact, null, 2),
        "application/json;charset=utf-8",
      );
      return true;
    }
    return false;
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
  // Lossy view dump, NOT a SessionArtifact — "Import conversation" rejects
  // it by design (only the server's round-trippable export restores).
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

// Trust-boundary envelope check for a user-picked file (CLAUDE.md §3): just
// enough to tell a SessionArtifact apart from a local-fallback export or a
// random JSON, with a friendly error instead of a server invalid_params.
// The server remains the authority on the full shape.
const artifactEnvelope = z.looseObject({
  version: z.literal(1),
  session: z.looseObject({ id: z.string().min(1) }),
  messages: z.array(z.unknown()),
  runs: z.array(z.unknown()),
  items: z.array(z.unknown()),
});

function pickFile(): Promise<string | null> {
  return new Promise((resolve) => {
    const input = document.createElement("input");
    input.type = "file";
    input.accept = "application/json,.json";
    input.onchange = () => {
      const file = input.files?.[0];
      if (!file) return resolve(null);
      void file.text().then(resolve, () => resolve(null));
    };
    // WebKit fires no event on a cancelled picker; the promise just stays
    // pending and the detached input is GC'd — harmless for a command.
    input.click();
  });
}

/** sessions.import — restore semantics: the session reappears under its
 *  original id (overwriting any current history), so after importing a
 *  session that's currently mounted we rebuild its view from the server. */
async function importConversation(): Promise<void> {
  if (!serverFeature("sessionExport")) {
    notifyError(t("convExport.importUnsupported"), { source: "import" });
    return;
  }
  const text = await pickFile();
  if (text === null) return;
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch {
    notifyError(t("convExport.notJson"), { source: "import" });
    return;
  }
  if (!artifactEnvelope.safeParse(raw).success) {
    notifyError(t("convExport.notLyra"), {
      source: "import",
    });
    return;
  }
  try {
    // Envelope-checked above; the server is the authority on the full shape.
    const { session } = await getContainer()
      .client()
      .sessions.import(raw as SessionArtifact);
    // Imported over a session that's currently mounted → its view is stale;
    // rebuild it from the restored server history.
    await rehydrateSessionView(session.id);
    useSessionStore.getState().selectTab(session.id);
    // projects too: the restored session's cwd may mint a project node.
    void invalidateSessions({ projects: true });
    toast.success(t("convExport.importSuccess", { title: session.title ?? session.id }));
  } catch (err) {
    console.error("[import] sessions.import failed:", err);
    notifyError(t("convExport.importFailed"), { source: "import" });
  }
}

export default definePlugin({
  name: "lyra.builtin.conversation-export",
  version: "1.0.0",
  setup({ host }) {
    host.commands.register({
      id: "chat.export.markdown",
      label: t("convExport.markdown"),
      icon: "filetext",
      group: "Chat",
      keywords: ["save", "download", "export"],
      run: async () => {
        if (!(await exportServer("md"))) exportMarkdown();
      },
    });
    host.commands.register({
      id: "chat.export.json",
      label: t("convExport.json"),
      icon: "code",
      group: "Chat",
      keywords: ["save", "download", "export", "archive"],
      run: async () => {
        if (!(await exportServer("json"))) exportJson();
      },
    });
    host.commands.register({
      id: "chat.import.json",
      label: t("convExport.import"),
      icon: "history",
      group: "Chat",
      keywords: ["restore", "load", "import"],
      run: () => importConversation(),
    });
  },
});
