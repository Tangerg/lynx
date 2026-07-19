// Wails desktop-shell asset endpoints — NOT the Lyra Runtime Protocol.
//
// The Go shell (app.go) serves the sideloaded-plugin manifest + bundles at
// `${baseUrl}/plugins`. That's host / packaging metadata, deliberately OUTSIDE
// the JSON-RPC protocol AND the sidecar (TRANSPORT.md §12 caps the sidecar at
// /v2/info + /v2/health/{live,ready}, and §6.1 forbids RESTy protocol shadows). It lives in
// rpc/ for one reason only: so EVERY outbound HTTP call goes through a single,
// injectable layer (ARCHITECTURE §10 — "协议是唯一 outbound 边界"), testable via
// the container's `setContainer()` seam. It is not, and must not become, a
// protocol method.

import { z } from "zod";
import { RpcTransportError } from "./errors";

/** One sideloaded plugin the shell advertises: a stable id + the path (relative
 *  to baseUrl) of its ESM entry bundle. */
export interface SideloadEntry {
  id: string;
  url: string;
}

// The manifest is an external HTTP response (trust boundary, CLAUDE.md §3) —
// validate it before the ids/urls are used to build dynamic-import URLs.
const SideloadListSchema = z.array(z.object({ id: z.string().min(1), url: z.string().min(1) }));

export interface ShellClientConfig {
  baseUrl: string;
  fetch?: typeof fetch;
}

export interface ShellClient {
  /** The desktop shell's sideloaded-plugin manifest (`GET ${baseUrl}/plugins`). */
  sideloadManifest(signal?: AbortSignal): Promise<SideloadEntry[]>;
}

export function createShellClient(config: ShellClientConfig): ShellClient {
  const baseUrl = config.baseUrl.replace(/\/+$/, "");
  const fetchImpl = config.fetch ?? globalThis.fetch.bind(globalThis);

  return {
    async sideloadManifest(signal) {
      let res: Response;
      try {
        res = await fetchImpl(`${baseUrl}/plugins`, {
          method: "GET",
          headers: { Accept: "application/json" },
          signal,
        });
      } catch (err) {
        throw new RpcTransportError(`shell /plugins: ${(err as Error).message}`);
      }
      if (!res.ok) throw new RpcTransportError(`shell /plugins: http ${res.status}`, res.status);
      let json: unknown;
      try {
        json = JSON.parse(await res.text());
      } catch (err) {
        throw new RpcTransportError(`shell /plugins: invalid JSON: ${(err as Error).message}`);
      }
      const parsed = SideloadListSchema.safeParse(json);
      if (!parsed.success) {
        throw new RpcTransportError(`shell /plugins: malformed manifest: ${parsed.error.message}`);
      }
      return parsed.data;
    },
  };
}
