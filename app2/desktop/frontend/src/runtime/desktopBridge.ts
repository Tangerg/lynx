import {
  protocolVersion,
  type RuntimeConnection,
} from "@lyra/runtime-contract";

export interface DesktopBootstrap {
  runtime: RuntimeConnection;
}

export interface DesktopBinding {
  call(method: string, ...parameters: unknown[]): Promise<unknown>;
}

const bootstrapMethod = "main.DesktopHost.Bootstrap";
const chooseDirectoryMethod = "main.NativeHost.ChooseDirectory";

export type DirectorySelection =
  | { type: "selected"; path: string }
  | { type: "canceled" };

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function exactKeys(
  value: Record<string, unknown>,
  keys: readonly string[],
): boolean {
  return (
    Object.keys(value).length === keys.length &&
    keys.every((key) => key in value)
  );
}

function parseConnection(value: unknown): RuntimeConnection {
  if (
    !isRecord(value) ||
    !exactKeys(value, [
      "endpoint",
      "localToken",
      "instanceId",
      "protocolVersion",
      "idempotencyNamespace",
      "generation",
    ]) ||
    typeof value.endpoint !== "string" ||
    typeof value.localToken !== "string" ||
    value.localToken.length === 0 ||
    typeof value.instanceId !== "string" ||
    value.instanceId.length === 0 ||
    value.protocolVersion !== protocolVersion ||
    typeof value.idempotencyNamespace !== "string" ||
    value.idempotencyNamespace.length === 0 ||
    !Number.isSafeInteger(value.generation) ||
    (value.generation as number) < 1
  ) {
    throw new TypeError("Desktop returned an invalid Runtime bootstrap");
  }
  const endpoint = new URL(value.endpoint);
  if (
    endpoint.protocol !== "http:" ||
    !["127.0.0.1", "[::1]"].includes(endpoint.hostname) ||
    endpoint.pathname !== "/" ||
    endpoint.search !== "" ||
    endpoint.hash !== "" ||
    endpoint.username !== "" ||
    endpoint.password !== ""
  ) {
    throw new TypeError("Desktop returned a non-loopback Runtime endpoint");
  }
  return {
    endpoint: endpoint.origin,
    localToken: value.localToken,
    instanceId: value.instanceId,
    protocolVersion,
    idempotencyNamespace: value.idempotencyNamespace,
    generation: value.generation as number,
  };
}

async function defaultBinding(): Promise<DesktopBinding> {
  if (!("_wails" in globalThis)) {
    throw new Error("Lyra Desktop host is unavailable");
  }
  const { Call } = await import("@wailsio/runtime");
  return {
    call: (method, ...parameters) => Call.ByName(method, ...parameters),
  };
}

export async function loadDesktopBootstrap(
  binding?: DesktopBinding,
): Promise<DesktopBootstrap> {
  const activeBinding = binding ?? (await defaultBinding());
  const value: unknown = await activeBinding.call(bootstrapMethod);
  if (!isRecord(value) || !exactKeys(value, ["runtime"])) {
    throw new TypeError("Desktop returned an invalid bootstrap envelope");
  }
  return { runtime: parseConnection(value.runtime) };
}

export async function chooseDirectory(
  binding?: DesktopBinding,
): Promise<DirectorySelection> {
  const activeBinding = binding ?? (await defaultBinding());
  const value: unknown = await activeBinding.call(chooseDirectoryMethod);
  if (!isRecord(value) || typeof value.type !== "string") {
    throw new TypeError("Desktop returned an invalid directory selection");
  }
  if (value.type === "canceled" && exactKeys(value, ["type"])) {
    return { type: "canceled" };
  }
  if (
    value.type === "selected" &&
    exactKeys(value, ["type", "path"]) &&
    typeof value.path === "string" &&
    value.path === value.path.trim() &&
    isAbsoluteFilesystemPath(value.path)
  ) {
    return { type: "selected", path: value.path };
  }
  throw new TypeError("Desktop returned an invalid directory selection");
}

function isAbsoluteFilesystemPath(path: string): boolean {
  return (
    path.startsWith("/") ||
    /^[A-Za-z]:[\\/]/.test(path) ||
    path.startsWith("\\\\")
  );
}
