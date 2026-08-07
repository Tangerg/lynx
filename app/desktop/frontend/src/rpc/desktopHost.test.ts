import { describe, expect, it, vi } from "vitest";
import { RpcTransportError } from "./errors";
import { createDesktopHostClient, type DesktopHostBinding } from "./desktopHost";

const BOOTSTRAP = "main.DesktopHost.Bootstrap";
const WINDOW_CHROME = "main.DesktopHost.WindowChrome";

/** A host that answers both methods, dispatching on the name the client asks for — which
 *  is the thing worth exercising: one call channel, two names. */
function hostBinding(
  answers: Partial<Record<string, () => Promise<unknown>>> = {},
): DesktopHostBinding & { call: ReturnType<typeof vi.fn> } {
  const defaults: Record<string, () => Promise<unknown>> = {
    [BOOTSTRAP]: async () => ({
      localRuntime: { endpoint: "http://127.0.0.1:17171" },
      sideloadedPlugins: [],
      sideloadIssues: [],
    }),
    [WINDOW_CHROME]: async () => ({
      controlsCentreY: 20,
      controlsInlineEnd: 72,
      measured: true,
    }),
  };
  const routes = { ...defaults, ...answers };
  return {
    call: vi.fn(async (method: string) => {
      const answer = routes[method];
      if (!answer) throw new Error(`unbound method ${method}`);
      return answer();
    }),
  };
}

describe("DesktopHostClient", () => {
  it("validates and caches the Wails bootstrap result", async () => {
    const binding = hostBinding({
      [BOOTSTRAP]: async () => ({
        localRuntime: { endpoint: "http://127.0.0.1:17171", localToken: "token" },
        sideloadedPlugins: [{ id: "acme.tools", source: "export default {};" }],
        sideloadIssues: [],
      }),
    });
    const client = createDesktopHostClient(binding);

    await expect(client.bootstrap()).resolves.toMatchObject({
      localRuntime: { localToken: "token" },
      sideloadedPlugins: [{ id: "acme.tools" }],
    });
    await client.bootstrap();
    expect(binding.call).toHaveBeenCalledTimes(1);
    expect(binding.call).toHaveBeenCalledWith(BOOTSTRAP);
  });

  it("returns null outside Wails", async () => {
    await expect(createDesktopHostClient(undefined).bootstrap()).resolves.toBeNull();
  });

  it("rejects a malformed host response", async () => {
    const client = createDesktopHostClient(
      hostBinding({ [BOOTSTRAP]: async () => ({ localRuntime: {} }) }),
    );
    await expect(client.bootstrap()).rejects.toBeInstanceOf(RpcTransportError);
  });

  it("reads the platform's window-control geometry", async () => {
    const binding = hostBinding();
    await expect(createDesktopHostClient(binding).windowChrome()).resolves.toEqual({
      controlsCentreY: 20,
      controlsInlineEnd: 72,
    });
    expect(binding.call).toHaveBeenCalledWith(WINDOW_CHROME);
  });

  // Every way of having no geometry has to arrive as the same `null`, because the
  // caller's only correct response is to leave the stylesheet's own numbers standing.
  // A zero or a partial object reaching the layout would collapse the gutter that
  // holds the window's controls clear of the header.
  it("answers null for every way of having nothing to measure", async () => {
    await expect(createDesktopHostClient(undefined).windowChrome()).resolves.toBeNull();

    const unmeasured = hostBinding({
      [WINDOW_CHROME]: async () => ({ controlsCentreY: 0, controlsInlineEnd: 0, measured: false }),
    });
    await expect(createDesktopHostClient(unmeasured).windowChrome()).resolves.toBeNull();

    const malformed = hostBinding({
      [WINDOW_CHROME]: async () => ({ controlsCentreY: 20 }),
    });
    await expect(createDesktopHostClient(malformed).windowChrome()).resolves.toBeNull();

    const broken = hostBinding({
      [WINDOW_CHROME]: async () => {
        throw new Error("no window");
      },
    });
    await expect(createDesktopHostClient(broken).windowChrome()).resolves.toBeNull();
  });

  // The name the runtime resolves against Go's reflection. Getting it wrong is a runtime
  // "unknown bound method" and nothing earlier, so it is worth stating once.
  it("addresses the host by its fully qualified Go method names", async () => {
    const binding = hostBinding();
    const client = createDesktopHostClient(binding);
    await client.bootstrap();
    await client.windowChrome();
    expect(binding.call.mock.calls.flat()).toEqual([BOOTSTRAP, WINDOW_CHROME]);
  });
});
