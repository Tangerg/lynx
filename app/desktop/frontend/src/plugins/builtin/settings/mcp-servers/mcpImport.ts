// Parser for the Claude Desktop `mcpServers` config block (the de-facto MCP
// interchange shape) → our ConfigureMCPServerRequest. Pasted JSON is an
// EXTERNAL trust boundary (§3), so it's validated with Zod before any field is
// trusted; everything downstream (the configure call) gets a well-typed value.
//
// Accepted per-server shape (lenient — different tools emit subsets):
//   { "command": "...", "args": [...], "env": {KEY:val} | ["KEY=val"], "type"? }
//   { "url": "...", "type"?, "headers"? | "authorization"? }
// `type` is optional: absent → inferred (command ⇒ stdio, url ⇒ http).

import { z } from "zod";
import type { ConfigureMCPServerRequest } from "@/rpc";

// env may be a map ({KEY: "val"}) or already the "KEY=value" array we store.
const envSchema = z
  .union([z.record(z.string(), z.string()), z.array(z.string())])
  .optional()
  .transform((env) =>
    env === undefined
      ? undefined
      : Array.isArray(env)
        ? env
        : Object.entries(env).map(([k, v]) => `${k}=${v}`),
  );

const serverSchema = z.object({
  type: z.enum(["stdio", "http", "sse"]).optional(),
  command: z.string().optional(),
  args: z.array(z.string()).optional(),
  env: envSchema,
  dir: z.string().optional(),
  cwd: z.string().optional(), // Claude Desktop's name for the working dir
  url: z.string().optional(),
  // A bearer token may arrive bare or wrapped in a Headers map; pull the
  // Authorization header and strip a leading "Bearer ".
  authorization: z.string().optional(),
  headers: z.record(z.string(), z.string()).optional(),
});

const rootSchema = z.object({
  mcpServers: z.record(z.string(), serverSchema),
});

function bearerFrom(s: z.infer<typeof serverSchema>): string | undefined {
  const raw = s.authorization ?? s.headers?.Authorization ?? s.headers?.authorization ?? undefined;
  if (raw === undefined) return undefined;
  return raw.replace(/^Bearer\s+/i, "");
}

export interface McpImportResult {
  configs: ConfigureMCPServerRequest[];
}

/**
 * Parse a Claude-Desktop-format JSON string into configure requests, one per
 * named server. Throws on malformed JSON or a server entry that matches
 * neither transport (no command and no url) — the caller surfaces the message.
 */
export function parseMcpImport(text: string): McpImportResult {
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch {
    throw new Error("Not valid JSON");
  }
  const parsed = rootSchema.safeParse(raw);
  if (!parsed.success) {
    throw new Error('Expected {"mcpServers": { "<name>": { … } }}');
  }
  const configs: ConfigureMCPServerRequest[] = [];
  for (const [name, s] of Object.entries(parsed.data.mcpServers)) {
    // sse collapses to http on our side (one streamable-HTTP transport).
    const transport = s.type
      ? s.type === "stdio"
        ? "stdio"
        : "http"
      : s.command
        ? "stdio"
        : s.url
          ? "http"
          : undefined;
    if (transport === undefined) {
      throw new Error(`Server "${name}" has neither a command (stdio) nor a url (http)`);
    }
    if (transport === "stdio") {
      configs.push({
        name,
        transport,
        enabled: true,
        command: s.command,
        args: s.args,
        env: s.env,
        dir: s.dir ?? s.cwd,
      });
    } else {
      configs.push({
        name,
        transport,
        enabled: true,
        url: s.url,
        authorization: bearerFrom(s),
      });
    }
  }
  if (configs.length === 0) throw new Error("No servers found under mcpServers");
  return { configs };
}
