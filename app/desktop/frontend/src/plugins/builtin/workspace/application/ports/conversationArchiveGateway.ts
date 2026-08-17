export type ConversationExportFormat = "md" | "json";

export type ConversationExportResult =
  { format: "md"; markdown?: string } | { format: "json"; artifact?: unknown };

export interface ImportedConversation {
  id: string;
  title?: string;
}

export interface ConversationArchiveGateway {
  exportConversation(
    sessionId: string,
    format: ConversationExportFormat,
  ): Promise<ConversationExportResult>;
  importConversation(artifact: unknown): Promise<ImportedConversation>;
}
