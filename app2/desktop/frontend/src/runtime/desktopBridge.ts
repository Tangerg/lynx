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

export type DesktopBridgeErrorCode =
  | "invalidRuntimeBootstrap"
  | "unsafeRuntimeEndpoint"
  | "hostUnavailable"
  | "invalidBootstrapEnvelope"
  | "invalidRemoteRuntimeState"
  | "inconsistentRemoteRuntimeState"
  | "invalidDirectorySelection"
  | "invalidImageSaveResult"
  | "invalidSessionArtifactSelection"
  | "invalidSessionExportResult";

export class DesktopBridgeError extends TypeError {
  constructor(readonly code: DesktopBridgeErrorCode) {
    super(code);
    this.name = "DesktopBridgeError";
  }
}

const bootstrapMethod = "main.DesktopHost.Bootstrap";
const chooseDirectoryMethod = "main.NativeHost.ChooseDirectory";
const saveImageMethod = "main.NativeHost.SaveImage";
const openSessionArtifactMethod = "main.NativeHost.OpenSessionArtifact";
const saveSessionExportMethod = "main.NativeHost.SaveSessionExport";
const remoteRuntimeMethod = "main.DesktopHost.RemoteRuntime";
const connectRemoteRuntimeMethod = "main.DesktopHost.ConnectRemoteRuntime";
const useLocalRuntimeMethod = "main.DesktopHost.UseLocalRuntime";
const useRemoteRuntimeMethod = "main.DesktopHost.UseRemoteRuntime";
const forgetRemoteRuntimeMethod = "main.DesktopHost.ForgetRemoteRuntime";

export type DirectorySelection =
  | { type: "selected"; path: string }
  | { type: "canceled" };

export type SessionArtifactSelection =
  | { type: "selected"; contents: string }
  | { type: "canceled" };

export type SessionExportSaveResult =
  | { type: "saved" }
  | { type: "canceled" };

export type ImageSaveResult = { type: "saved" } | { type: "canceled" };

export interface RemoteRuntimeState {
	configured: boolean;
	active: boolean;
	connected: boolean;
	endpoint?: string;
	serverName?: string;
	detail?: string;
}

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
      "bearerToken",
      "instanceId",
      "protocolVersion",
      "idempotencyNamespace",
      "generation",
    ]) ||
    typeof value.endpoint !== "string" ||
    typeof value.bearerToken !== "string" ||
    value.bearerToken.length === 0 ||
    typeof value.instanceId !== "string" ||
    value.instanceId.length === 0 ||
    value.protocolVersion !== protocolVersion ||
    typeof value.idempotencyNamespace !== "string" ||
    value.idempotencyNamespace.length === 0 ||
    !Number.isSafeInteger(value.generation) ||
    (value.generation as number) < 1
  ) {
    throw new DesktopBridgeError("invalidRuntimeBootstrap");
  }
  const endpoint = new URL(value.endpoint);
  const local = endpoint.protocol === "http:" && ["127.0.0.1", "[::1]"].includes(endpoint.hostname);
  const remote = endpoint.protocol === "https:" && endpoint.hostname !== "";
  if (
	(!local && !remote) ||
    endpoint.pathname !== "/" ||
    endpoint.search !== "" ||
    endpoint.hash !== "" ||
    endpoint.username !== "" ||
    endpoint.password !== ""
  ) {
	throw new DesktopBridgeError("unsafeRuntimeEndpoint");
  }
  return {
    endpoint: endpoint.origin,
    bearerToken: value.bearerToken,
    instanceId: value.instanceId,
    protocolVersion,
    idempotencyNamespace: value.idempotencyNamespace,
    generation: value.generation as number,
  };
}

async function defaultBinding(): Promise<DesktopBinding> {
  if (!("_wails" in globalThis)) {
    throw new DesktopBridgeError("hostUnavailable");
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
    throw new DesktopBridgeError("invalidBootstrapEnvelope");
  }
  return { runtime: parseConnection(value.runtime) };
}

export async function remoteRuntimeState(binding?: DesktopBinding) {
	const activeBinding = binding ?? (await defaultBinding());
	return parseRemoteRuntimeState(await activeBinding.call(remoteRuntimeMethod));
}

export async function connectRemoteRuntime(
	endpoint: string,
	token: string,
	binding?: DesktopBinding,
) {
	const activeBinding = binding ?? (await defaultBinding());
	return parseRemoteRuntimeState(
		await activeBinding.call(connectRemoteRuntimeMethod, endpoint, token),
	);
}

export async function useLocalRuntime(binding?: DesktopBinding) {
	const activeBinding = binding ?? (await defaultBinding());
	return parseRemoteRuntimeState(await activeBinding.call(useLocalRuntimeMethod));
}

export async function useRemoteRuntime(binding?: DesktopBinding) {
	const activeBinding = binding ?? (await defaultBinding());
	return parseRemoteRuntimeState(await activeBinding.call(useRemoteRuntimeMethod));
}

export async function forgetRemoteRuntime(binding?: DesktopBinding) {
	const activeBinding = binding ?? (await defaultBinding());
	return parseRemoteRuntimeState(await activeBinding.call(forgetRemoteRuntimeMethod));
}

function parseRemoteRuntimeState(value: unknown): RemoteRuntimeState {
	const allowedKeys = new Set([
		"configured",
		"active",
		"connected",
		"endpoint",
		"serverName",
		"detail",
	]);
	if (
		!isRecord(value) ||
		!["configured", "active", "connected"].every((key) => key in value) ||
		!Object.keys(value).every((key) => allowedKeys.has(key)) ||
		typeof value.configured !== "boolean" ||
		typeof value.active !== "boolean" ||
		typeof value.connected !== "boolean" ||
		(value.endpoint !== undefined && typeof value.endpoint !== "string") ||
		(value.serverName !== undefined && typeof value.serverName !== "string") ||
		(value.detail !== undefined && typeof value.detail !== "string")
	) {
		throw new DesktopBridgeError("invalidRemoteRuntimeState");
	}
	if (
		(value.active && !value.configured) ||
		(value.connected && !value.active) ||
		value.configured !== (value.endpoint !== undefined) ||
		value.configured !== (value.serverName !== undefined) ||
		(value.endpoint !== undefined && !isSafeRemoteOrigin(value.endpoint)) ||
		(value.serverName !== undefined &&
			(value.serverName.trim() !== value.serverName || value.serverName === "")) ||
		(value.detail !== undefined &&
			(value.detail.trim() !== value.detail || value.detail.length > 4_096))
	) {
		throw new DesktopBridgeError("inconsistentRemoteRuntimeState");
	}
	return value as unknown as RemoteRuntimeState;
}

function isSafeRemoteOrigin(value: string) {
	try {
		const endpoint = new URL(value);
		return (
			endpoint.protocol === "https:" &&
			value.trim() === value &&
			endpoint.hostname !== "" &&
			endpoint.pathname === "/" &&
			endpoint.search === "" &&
			endpoint.hash === "" &&
			endpoint.username === "" &&
			endpoint.password === ""
		);
	} catch {
		return false;
	}
}

export async function chooseDirectory(
  binding?: DesktopBinding,
): Promise<DirectorySelection> {
  const activeBinding = binding ?? (await defaultBinding());
  const value: unknown = await activeBinding.call(chooseDirectoryMethod);
  if (!isRecord(value) || typeof value.type !== "string") {
    throw new DesktopBridgeError("invalidDirectorySelection");
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
  throw new DesktopBridgeError("invalidDirectorySelection");
}

export async function saveImage(
  source: string,
  binding?: DesktopBinding,
): Promise<ImageSaveResult> {
  const activeBinding = binding ?? (await defaultBinding());
  const value: unknown = await activeBinding.call(saveImageMethod, source);
  if (
    !isRecord(value) ||
    !exactKeys(value, ["type"]) ||
    (value.type !== "saved" && value.type !== "canceled")
  ) {
    throw new DesktopBridgeError("invalidImageSaveResult");
  }
  return { type: value.type };
}

export async function openSessionArtifact(
  binding?: DesktopBinding,
): Promise<SessionArtifactSelection> {
  const activeBinding = binding ?? (await defaultBinding());
  const value: unknown = await activeBinding.call(openSessionArtifactMethod);
  if (!isRecord(value) || typeof value.type !== "string") {
    throw new DesktopBridgeError("invalidSessionArtifactSelection");
  }
  if (value.type === "canceled" && exactKeys(value, ["type"])) {
    return { type: "canceled" };
  }
  if (
    value.type === "selected" &&
    exactKeys(value, ["type", "contents"]) &&
    typeof value.contents === "string" &&
    value.contents.length > 0
  ) {
    return { type: "selected", contents: value.contents };
  }
  throw new DesktopBridgeError("invalidSessionArtifactSelection");
}

export async function saveSessionExport(
  sessionId: string,
  format: "json" | "md",
  contents: string,
  binding?: DesktopBinding,
): Promise<SessionExportSaveResult> {
  const activeBinding = binding ?? (await defaultBinding());
  const value: unknown = await activeBinding.call(
    saveSessionExportMethod,
    sessionId,
    format,
    contents,
  );
  if (
    !isRecord(value) ||
    !exactKeys(value, ["type"]) ||
    (value.type !== "saved" && value.type !== "canceled")
  ) {
    throw new DesktopBridgeError("invalidSessionExportResult");
  }
  return { type: value.type };
}

function isAbsoluteFilesystemPath(path: string): boolean {
  return (
    path.startsWith("/") ||
    /^[A-Za-z]:[\\/]/.test(path) ||
    path.startsWith("\\\\")
  );
}
