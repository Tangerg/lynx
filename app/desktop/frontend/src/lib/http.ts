import ky, { type KyInstance } from "ky";
import { getConfig } from "@/plugins/sdk/config";
import { listRpcAfterHooks, listRpcBeforeHooks } from "@/plugins/sdk/selectors";

// Default base URL for the local Go AG-UI mock. Plugins (typically
// `lyra.builtin.default-config`) can override via
// `host.config.set("api.baseUrl", "...")`. The override is read on every
// request so a runtime change takes effect immediately.
export const AGUI_BASE = "http://127.0.0.1:17171";

function activeBase(): string {
  return getConfig<string>("api.baseUrl") ?? AGUI_BASE;
}

const beforeRequest = async (state: { request: Request }) => {
  for (const hook of listRpcBeforeHooks()) {
    const result = await hook(state.request);
    if (result instanceof Request) return result;
  }
};
const afterResponse = async (state: { request: Request; response: Response }) => {
  for (const hook of listRpcAfterHooks()) {
    const result = await hook(state.request, state.response);
    if (result instanceof Response) return result;
  }
};

// Re-anchor `req` against `base`, preserving path + search + the original
// Request's headers/body. Returns `req` unchanged when already on the right
// host so this is cheap on the hot path.
function anchorOn(req: Request, base: string): Request {
  if (req.url.startsWith(base)) return req;
  const { pathname, search } = new URL(req.url);
  const baseSlash = base.endsWith("/") ? base : base + "/";
  const target = new URL(pathname.replace(/^\//, "") + search, baseSlash);
  return new Request(target.toString(), req);
}

const rewriteBase = (state: { request: Request }) => anchorOn(state.request, activeBase());

// Pre-configured ky instance for the local backend.
//
// `retry: 0` because react-query already handles retries — letting ky
// retry would double-retry on transient failures.
export const api: KyInstance = ky.create({
  baseUrl: AGUI_BASE,
  retry: 0,
  timeout: 30_000,
  headers: { Accept: "application/json" },
  hooks: {
    beforeRequest: [rewriteBase, beforeRequest],
    afterResponse: [afterResponse],
  },
});
