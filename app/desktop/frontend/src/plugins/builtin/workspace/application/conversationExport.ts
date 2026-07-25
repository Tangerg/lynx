import type { Message } from "@/plugins/builtin/agent/public/viewState";
import { fileTransfer } from "./ports/fileTransfer";
import { formatDateTime } from "@/lib/i18n/relativeTime";
import { toast } from "sonner";
import { z } from "zod";
import { notifyError } from "@/plugins/sdk";
import { t } from "@/lib/i18n";
import { lookupExtensionByKey } from "@/plugins/sdk";
import { MESSAGE_ROLE } from "@/plugins/sdk/kernelPoints";
import { runtimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import { getActiveConversationSnapshot } from "@/plugins/builtin/agent/public/conversation";
import { flattenMarkdown } from "@/plugins/builtin/agent/public/messageContent";
import {
  getActiveSessionId,
  invalidateAgentSessions,
  rehydrateSessionView,
  selectAgentSession,
} from "@/plugins/builtin/agent/public/session";
import {
  conversationArchiveGateway,
  type ConversationExportFormat,
} from "./ports/conversationArchiveGateway";

function timestampForFilename(): string {
  return new Date().toISOString().replace(/[:.]/g, "-").slice(0, 19);
}

// The role's display name belongs to whoever contributed the role — MESSAGE_ROLE
// carries it, already localized. The fold used to bake a hardcoded English copy
// onto every message, duplicating both this registry and the locale catalog it
// reads from.
function roleDisplayName(role: Message["role"]): string {
  return lookupExtensionByKey(MESSAGE_ROLE, role)?.displayName ?? role;
}

function renderMessageMarkdown(msg: Message): string {
  const body = flattenMarkdown(msg.blocks).trim();
  if (!body) return "";
  const headerName = roleDisplayName(msg.role);
  return `## ${headerName} · ${formatDateTime(msg.createdAt)}\n\n${body}\n`;
}

async function exportServer(format: ConversationExportFormat): Promise<boolean> {
  const sid = getActiveSessionId();
  if (!sid || !runtimeCapability("sessionExport")) return false;
  try {
    const resp = await conversationArchiveGateway().exportConversation(sid, format);
    const stamp = timestampForFilename();
    if (resp.format === "md" && resp.markdown !== undefined) {
      fileTransfer().download(
        `lyra-${sid}-${stamp}.md`,
        resp.markdown,
        "text/markdown;charset=utf-8",
      );
      return true;
    }
    if (resp.format === "json" && resp.artifact) {
      fileTransfer().download(
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
  fileTransfer().download(
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
  fileTransfer().download(
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

export async function exportConversationMarkdown(): Promise<void> {
  if (!(await exportServer("md"))) exportLocalMarkdown();
}

export async function exportConversationJson(): Promise<void> {
  if (!(await exportServer("json"))) exportLocalJson();
}

export async function importConversationJson(): Promise<void> {
  if (!runtimeCapability("sessionExport")) {
    notifyError(t("convExport.importUnsupported"), { source: "import" });
    return;
  }
  const text = await fileTransfer().pickText("application/json,.json");
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
    const session = await conversationArchiveGateway().importConversation(raw);
    await rehydrateSessionView(session.id);
    selectAgentSession(session.id);
    void invalidateAgentSessions({ projects: true });
    toast.success(t("convExport.importSuccess", { title: session.title ?? session.id }));
  } catch (err) {
    console.error("[import] sessions.import failed:", err);
    notifyError(t("convExport.importFailed"), { source: "import" });
  }
}
