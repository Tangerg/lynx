import { RetirableTaskCohort } from "@/lib/taskQueue";
import { createPublicationSlot } from "@/lib/publicationSlot";
import { formatDateTime } from "@/lib/i18n/relativeTime";
import { t } from "@/lib/i18n";
import { getActiveConversationSnapshot } from "@/plugins/builtin/agent/public/conversation";
import { flattenMarkdown } from "@/plugins/builtin/agent/public/messageContent";
import {
  getActiveSessionId,
  invalidateAgentSessions,
  rehydrateSessionView,
  selectAgentSession,
} from "@/plugins/builtin/agent/public/session";
import { runtimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import { lookupExtensionByKey, notifyError } from "@/plugins/sdk";
import { MESSAGE_ROLE } from "@/plugins/sdk/kernelPoints";
import type { Message } from "@/plugins/builtin/agent/public/viewState";
import { toast } from "sonner";
import { z } from "zod";
import type {
  ConversationArchiveGateway,
  ConversationExportFormat,
  ConversationExportResult,
} from "./ports/conversationArchiveGateway";
import type { FileTransferPort } from "./ports/fileTransfer";

function timestampForFilename(date: Date): string {
  return date.toISOString().replace(/[:.]/g, "-").slice(0, 19);
}

// The role's display name belongs to whoever contributed the role — MESSAGE_ROLE
// carries the catalog key, resolved here at export time. The fold used to bake a
// hardcoded English copy onto every message, duplicating both this registry and
// the locale catalog it reads from; the registry then held resolved text, which
// froze it in the locale the app booted in.
function roleDisplayName(role: Message["role"]): string {
  const key = lookupExtensionByKey(MESSAGE_ROLE, role)?.displayName;
  return key ? t(key) : role;
}

function renderMessageMarkdown(msg: Message): string {
  const body = flattenMarkdown(msg.blocks).trim();
  if (!body) return "";
  const headerName = roleDisplayName(msg.role);
  return `## ${headerName} · ${formatDateTime(msg.createdAt)}\n\n${body}\n`;
}

interface LocalExportMaterial {
  readonly sessionId: string;
  readonly filename: string;
  readonly content: string;
  readonly mime: string;
}

/** Captures the exact Session projection selected when the command begins. */
function captureLocalExport(
  sessionId: string,
  format: ConversationExportFormat,
): LocalExportMaterial {
  const capturedAt = new Date();
  const stamp = timestampForFilename(capturedAt);
  const view = getActiveConversationSnapshot();
  if (format === "md") {
    const sections: string[] = [
      `# ${t("convExport.docTitle")} \`${sessionId}\``,
      `*${t("convExport.exportedAt", { time: capturedAt.toISOString() })}*`,
      "",
    ];
    for (const message of view.messages) {
      const rendered = renderMessageMarkdown(message);
      if (rendered) sections.push(rendered);
    }
    return {
      sessionId,
      filename: `lyra-${sessionId}-${stamp}.md`,
      content: sections.join("\n"),
      mime: "text/markdown;charset=utf-8",
    };
  }
  return {
    sessionId,
    filename: `lyra-${sessionId}-${stamp}.json`,
    content: JSON.stringify(
      {
        version: 1,
        sessionId,
        exportedAt: capturedAt.toISOString(),
        messages: view.messages,
        timeline: view.timeline,
        toolCalls: view.toolCalls,
      },
      null,
      2,
    ),
    mime: "application/json;charset=utf-8",
  };
}

function serverExportMaterial(
  local: LocalExportMaterial,
  format: ConversationExportFormat,
  response: ConversationExportResult,
): LocalExportMaterial | null {
  if (format === "md" && response.format === "md" && response.markdown !== undefined) {
    return { ...local, content: response.markdown };
  }
  if (format === "json" && response.format === "json" && response.artifact !== undefined) {
    const content = JSON.stringify(response.artifact, null, 2);
    return content === undefined ? null : { ...local, content };
  }
  return null;
}

const artifactEnvelope = z.looseObject({
  version: z.literal(1),
  session: z.looseObject({ id: z.string().min(1) }),
  messages: z.array(z.unknown()),
  runs: z.array(z.unknown()),
  items: z.array(z.unknown()),
});

class ConversationArchiveGenerationRetiredError extends Error {
  override readonly name = "ConversationArchiveGenerationRetiredError";

  constructor() {
    super("conversation_archive_generation_retired");
  }
}

class ConversationArchiveGeneration {
  readonly #gateway: ConversationArchiveGateway;
  readonly #files: FileTransferPort;
  readonly #cohort = new RetirableTaskCohort(new ConversationArchiveGenerationRetiredError());
  #importOperation: Promise<void> | null = null;

  constructor(gateway: ConversationArchiveGateway, files: FileTransferPort) {
    this.#gateway = gateway;
    this.#files = files;
  }

  async export(format: ConversationExportFormat): Promise<void> {
    try {
      const sessionId = getActiveSessionId();
      if (!sessionId) return;
      const local = captureLocalExport(sessionId, format);
      if (!runtimeCapability("sessionExport")) {
        this.#download(local);
        return;
      }

      let server: LocalExportMaterial | null = null;
      try {
        const response = await this.#execute(() =>
          this.#gateway.exportConversation(local.sessionId, format),
        );
        server = serverExportMaterial(local, format, response);
      } catch (error) {
        if (this.#cohort.retired) throw error;
        console.warn("[export] sessions.export failed; falling back to captured material:", error);
      }
      this.#download(server ?? local);
    } catch (error) {
      if (!this.#cohort.retired) throw error;
    }
  }

  importJson(): Promise<void> {
    if (this.#importOperation) return this.#importOperation;
    const operation = this.#runImport();
    this.#importOperation = operation;
    void operation.then(
      () => this.#releaseImport(operation),
      () => this.#releaseImport(operation),
    );
    return operation;
  }

  retire(): void {
    this.#cohort.retire();
    this.#importOperation = null;
  }

  async #runImport(): Promise<void> {
    try {
      if (!runtimeCapability("sessionExport")) {
        notifyError(t("convExport.importUnsupported"), { source: "import" });
        return;
      }
      const text = await this.#execute(() => this.#files.pickText("application/json,.json"));
      if (text === null) return;

      let artifact: unknown;
      try {
        artifact = JSON.parse(text);
      } catch {
        this.#cohort.assertCurrent();
        notifyError(t("convExport.notJson"), { source: "import" });
        return;
      }
      if (!artifactEnvelope.safeParse(artifact).success) {
        this.#cohort.assertCurrent();
        notifyError(t("convExport.notLyra"), { source: "import" });
        return;
      }

      const session = await this.#execute(() => this.#gateway.importConversation(artifact));
      await this.#execute(() => rehydrateSessionView(session.id));
      await this.#repairSessionList();
      this.#cohort.assertCurrent();
      selectAgentSession(session.id);
      toast.success(t("convExport.importSuccess", { title: session.title ?? session.id }));
    } catch (error) {
      if (this.#cohort.retired) return;
      console.error("[import] sessions.import failed:", error);
      this.#cohort.assertCurrent();
      notifyError(t("convExport.importFailed"), { source: "import" });
    }
  }

  async #execute<T>(operation: () => PromiseLike<T>): Promise<T> {
    this.#cohort.assertCurrent();
    const value = await this.#cohort.settle(operation());
    this.#cohort.assertCurrent();
    return value;
  }

  async #repairSessionList(): Promise<void> {
    try {
      await this.#execute(() => invalidateAgentSessions());
    } catch (error) {
      if (this.#cohort.retired) throw error;
      // The import response and rehydrated Session are authoritative. Runtime
      // events and the next Session collection read retain the repair path.
    }
  }

  #download(material: LocalExportMaterial): void {
    this.#cohort.assertCurrent();
    this.#files.download(material.filename, material.content, material.mime);
  }

  #releaseImport(operation: Promise<void>): void {
    if (this.#importOperation === operation) this.#importOperation = null;
  }
}

export interface ConversationArchiveOwnerDependencies {
  readonly gateway: ConversationArchiveGateway;
  readonly files: FileTransferPort;
}

/**
 * Owns one exact Plugin Host's conversation archive commands. A Runtime
 * generation replacement retires the whole command cohort — picker, RPC,
 * rehydrate, query repair, navigation, toast and download all share one owner.
 */
export class ConversationArchiveOwner {
  readonly #dependencies: ConversationArchiveOwnerDependencies;
  #generation: ConversationArchiveGeneration;
  #disposed = false;

  private constructor(dependencies: ConversationArchiveOwnerDependencies) {
    this.#dependencies = dependencies;
    this.#generation = this.#newGeneration();
  }

  static install(dependencies: ConversationArchiveOwnerDependencies): ConversationArchiveOwner {
    const owner = new ConversationArchiveOwner(dependencies);
    conversationArchivePublication.publish(owner, (predecessor) => predecessor.dispose());
    return owner;
  }

  static current(): ConversationArchiveOwner {
    const owner = conversationArchivePublication.current();
    if (!owner || owner.#disposed) throw new Error("Conversation archive owner is not installed");
    return owner;
  }

  export(format: ConversationExportFormat): Promise<void> {
    return this.#generation.export(format);
  }

  importJson(): Promise<void> {
    return this.#generation.importJson();
  }

  replaceRuntimeGeneration(): void {
    if (this.#disposed || !conversationArchivePublication.owns(this)) return;
    const predecessor = this.#generation;
    this.#generation = this.#newGeneration();
    predecessor.retire();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation.retire();
    conversationArchivePublication.withdraw(this);
  }

  #newGeneration(): ConversationArchiveGeneration {
    return new ConversationArchiveGeneration(this.#dependencies.gateway, this.#dependencies.files);
  }
}

const conversationArchivePublication = createPublicationSlot<ConversationArchiveOwner>();

export function exportConversationMarkdown(): Promise<void> {
  return ConversationArchiveOwner.current().export("md");
}

export function exportConversationJson(): Promise<void> {
  return ConversationArchiveOwner.current().export("json");
}

export function importConversationJson(): Promise<void> {
  return ConversationArchiveOwner.current().importJson();
}
