// Parser for the Claude Desktop `mcpServers` config block (the de-facto MCP
// interchange shape) → our settings MCP config input. Pasted JSON is an
// EXTERNAL trust boundary (§3), so it's validated with Zod before any field is
// trusted; everything downstream (the configure call) gets a well-typed value.
//
// Accepted per-server shape (lenient — different tools emit subsets):
//   { "command": "...", "args": [...], "env": {KEY:val} | ["KEY=val"], "type"? }
//   { "url": "...", "type"?, "headers"? | "authorization"?, "timeout"? }
// `type` is optional: absent → inferred (command ⇒ stdio, url ⇒ http). An
// Authorization header is split out to the dedicated bearer field; every OTHER
// header is preserved (no silent drop).

import { z } from "zod";
import type { MCPServerConfigInput } from "./mcpServerInput";

// env may be a map ({KEY:"val"}) or a "KEY=value" array; both normalize to the
// KEY→value map our config input carries (split on the FIRST '=' so a value may
// itself contain '=').
const envSchema = z
  .union([z.record(z.string(), z.string()), z.array(z.string())])
  .optional()
  .transform((env) => {
    if (env === undefined) return undefined;
    if (!Array.isArray(env)) return env;
    const out: Record<string, string> = {};
    for (const kv of env) {
      const i = kv.indexOf("=");
      if (i === -1) out[kv] = "";
      else out[kv.slice(0, i)] = kv.slice(i + 1);
    }
    return out;
  });

const serverSchema = z.object({
  // Standard mcpServers vocab — accept it all (stdio | streamableHttp | http |
  // sse | anything). stdio stays stdio; every url-based type collapses onto our
  // one remote transport (streamableHttp). Lenient string, not an enum, so a
  // novel type value pastes in rather than failing.
  type: z.string().optional(),
  command: z.string().optional(),
  args: z.array(z.string()).optional(),
  env: envSchema,
  dir: z.string().optional(),
  cwd: z.string().optional(), // Claude Desktop's name for the working dir
  url: z.string().optional(),
  // A bearer token may arrive bare or as a Headers "Authorization" entry; it's
  // pulled into the dedicated bearer field, the rest of headers is preserved.
  authorization: z.string().optional(),
  headers: z.record(z.string(), z.string()).optional(),
  timeout: z.number().optional(), // seconds (Cherry-style); 0/absent = unbounded
});

type ParsedServer = z.infer<typeof serverSchema>;

function bearerFrom(s: ParsedServer): string | undefined {
  const raw = s.authorization ?? s.headers?.Authorization ?? s.headers?.authorization ?? undefined;
  if (raw === undefined) return undefined;
  return raw.replace(/^Bearer\s+/i, "");
}

// headersExceptAuth keeps every imported header EXCEPT Authorization (which goes
// to the dedicated bearer field). Returns undefined when nothing remains, so an
// Authorization-only headers block doesn't store an empty map.
function headersExceptAuth(s: ParsedServer): Record<string, string> | undefined {
  if (!s.headers) return undefined;
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(s.headers)) {
    if (k.toLowerCase() === "authorization") continue;
    out[k] = v;
  }
  return Object.keys(out).length ? out : undefined;
}

export interface McpImportResult {
  configs: MCPServerConfigInput[];
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
  const parsed = z.object({ mcpServers: z.record(z.string(), serverSchema) }).safeParse(raw);
  if (!parsed.success) {
    throw new Error('Expected {"mcpServers": { "<name>": { … } }}');
  }
  const configs: MCPServerConfigInput[] = [];
  for (const [name, s] of Object.entries(parsed.data.mcpServers)) {
    // stdio stays stdio; any other (http / streamableHttp / sse / …) or a bare
    // url collapses onto streamableHttp — our one remote transport.
    const type = s.type
      ? s.type === "stdio"
        ? "stdio"
        : "streamableHttp"
      : s.command
        ? "stdio"
        : s.url
          ? "streamableHttp"
          : undefined;
    if (type === undefined) {
      throw new Error(`Server "${name}" has neither a command (stdio) nor a url (streamableHttp)`);
    }
    if (type === "stdio") {
      configs.push({
        name,
        transport: type,
        enabled: true,
        command: s.command,
        args: s.args,
        env: s.env,
        dir: s.dir ?? s.cwd,
        timeoutSeconds: s.timeout,
      });
    } else {
      configs.push({
        name,
        transport: type,
        enabled: true,
        url: s.url,
        authorization: bearerFrom(s),
        headers: headersExceptAuth(s),
        timeoutSeconds: s.timeout,
      });
    }
  }
  if (configs.length === 0) throw new Error("No servers found under mcpServers");
  return { configs };
}
