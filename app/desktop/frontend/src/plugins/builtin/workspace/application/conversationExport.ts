import type { Message } from "@/plugins/builtin/agent/public/viewState";
import type { SessionArtifact } from "@/rpc";
import { toast } from "sonner";
import { z } from "zod";
import { getContainer } from "@/main/container";
import { notifyError } from "@/lib/notify";
import { t } from "@/lib/i18n";
import { asSessionId } from "@/rpc";
import { serverFeature } from "@/state/runtimeStore";
import { invalidateSessions } from "@/lib/data/queries";
import { getActiveConversationSnapshot } from "@/plugins/builtin/agent/public/conversation";
import { flattenMarkdown } from "@/plugins/builtin/agent/public/messageContent";
import {
  getActiveSessionId,
  rehydrateSessionView,
  selectAgentSession,
} from "@/plugins/builtin/agent/public/session";

function timestampForFilename(): string {
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
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

function renderMessageMarkdown(msg: Message): string {
  const body = flattenMarkdown(msg.blocks).trim();
  if (!body) return "";
  const headerName = msg.who || msg.role;
  return `## ${headerName} · ${msg.time}\n\n${body}\n`;
}

async function exportServer(format: "md" | "json"): Promise<boolean> {
  const sid = getActiveSessionId();
  if (!sid || !serverFeature("sessionExport")) return false;
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
    console.warn("[export] sessions.export failed; falling back to local render:", err);
    return false;
  }
}

function exportLocalMarkdown(): void {
  const view = getActiveConversationSnapshot();
  const sid = getActiveSessionId();
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

function exportLocalJson(): void {
  const view = getActiveConversationSnapshot();
  const sid = getActiveSessionId();
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
    input.click();
  });
}

export async function exportConversationMarkdown(): Promise<void> {
  if (!(await exportServer("md"))) exportLocalMarkdown();
}

export async function exportConversationJson(): Promise<void> {
  if (!(await exportServer("json"))) exportLocalJson();
}

export async function importConversationJson(): Promise<void> {
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
    notifyError(t("convExport.notLyra"), { source: "import" });
    return;
  }
  try {
    const { session } = await getContainer()
      .client()
      .sessions.import(raw as SessionArtifact);
    await rehydrateSessionView(session.id);
    selectAgentSession(session.id);
    void invalidateSessions({ projects: true });
    toast.success(t("convExport.importSuccess", { title: session.title ?? session.id }));
  } catch (err) {
    console.error("[import] sessions.import failed:", err);
    notifyError(t("convExport.importFailed"), { source: "import" });
  }
}
