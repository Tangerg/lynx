import { z } from "zod";
import { toolCatalogGateway } from "./ports/toolCatalogGateway";
import type { InvokeDiagnosticToolInput } from "./ports/toolCatalogGateway";

const argumentsSchema = z.record(z.string(), z.unknown());

export type DiagnosticArgumentsParseResult =
  | { ok: true; value: Record<string, unknown> }
  | { ok: false; reason: "invalidJson" | "objectRequired" };

export function parseDiagnosticToolArguments(text: string): DiagnosticArgumentsParseResult {
  let value: unknown;
  try {
    value = JSON.parse(text);
  } catch {
    return { ok: false, reason: "invalidJson" };
  }
  const parsed = argumentsSchema.safeParse(value);
  return parsed.success
    ? { ok: true, value: parsed.data }
    : { ok: false, reason: "objectRequired" };
}

export function invokeDiagnosticTool(input: InvokeDiagnosticToolInput): Promise<unknown> {
  return toolCatalogGateway().invokeDiagnosticTool(input);
}

export function formatDiagnosticToolResult(value: unknown): string {
  const encoded = JSON.stringify(value, null, 2);
  return encoded === undefined ? String(value) : encoded;
}
