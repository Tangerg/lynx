import { z } from "zod";
import type {
  DiagnosticToolGateway,
  InvokeDiagnosticToolInput,
} from "./ports/diagnosticToolGateway";

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

class DiagnosticToolGenerationRetiredError extends Error {
  override readonly name = "DiagnosticToolGenerationRetiredError";

  constructor() {
    super("diagnostic_tool_generation_retired");
  }
}

class DiagnosticToolGeneration {
  readonly #gateway: DiagnosticToolGateway;
  readonly #retiredError = new DiagnosticToolGenerationRetiredError();
  readonly #settlers = new Set<() => void>();
  readonly #tails = new Map<string, Promise<void>>();
  #retired = false;

  constructor(gateway: DiagnosticToolGateway) {
    this.#gateway = gateway;
  }

  invoke(input: InvokeDiagnosticToolInput): Promise<unknown> {
    const identity = `${input.cwd ?? ""}\u0000${input.name}`;
    const result = this.#settle(this.#tails.get(identity) ?? Promise.resolve()).then(async () => {
      this.#assertCurrent();
      const value = await this.#settle(this.#gateway.invoke(input));
      this.#assertCurrent();
      return value;
    });
    const settlement = result.then(
      () => undefined,
      () => undefined,
    );
    this.#tails.set(identity, settlement);
    void settlement.then(() => {
      if (this.#tails.get(identity) === settlement) this.#tails.delete(identity);
    });
    return result;
  }

  retire(): void {
    if (this.#retired) return;
    this.#retired = true;
    for (const settle of [...this.#settlers]) settle();
    this.#settlers.clear();
    this.#tails.clear();
  }

  #settle<T>(operation: Promise<T>): Promise<T> {
    this.#assertCurrent();
    return new Promise<T>((resolve, reject) => {
      let pending = true;
      const finish = () => {
        if (!pending) return false;
        pending = false;
        this.#settlers.delete(retire);
        return true;
      };
      const retire = () => {
        if (finish()) reject(this.#retiredError);
      };
      this.#settlers.add(retire);
      operation.then(
        (value) => {
          if (finish()) resolve(value);
        },
        (error: unknown) => {
          if (finish()) reject(error);
        },
      );
      if (this.#retired) retire();
    });
  }

  #assertCurrent(): void {
    if (this.#retired) throw this.#retiredError;
  }
}

/** Owns direct Tool invocations for one exact Plugin Host and Runtime generation. */
export class DiagnosticToolOwner {
  static #active: DiagnosticToolOwner | null = null;
  static #materialGeneration = 0;
  static readonly #listeners = new Set<() => void>();

  readonly #gateway: DiagnosticToolGateway;
  #generation: DiagnosticToolGeneration;
  #disposed = false;

  private constructor(gateway: DiagnosticToolGateway) {
    this.#gateway = gateway;
    this.#generation = new DiagnosticToolGeneration(gateway);
  }

  static install(gateway: DiagnosticToolGateway): DiagnosticToolOwner {
    const predecessor = DiagnosticToolOwner.#active;
    const owner = new DiagnosticToolOwner(gateway);
    DiagnosticToolOwner.#active = owner;
    predecessor?.dispose();
    DiagnosticToolOwner.#advanceMaterialGeneration();
    return owner;
  }

  static current(): DiagnosticToolOwner {
    const owner = DiagnosticToolOwner.#active;
    if (!owner || owner.#disposed) throw new Error("Diagnostic Tool owner is not installed");
    return owner;
  }

  static materialGeneration(): number {
    return DiagnosticToolOwner.#materialGeneration;
  }

  static subscribeMaterialGeneration(listener: () => void): () => void {
    DiagnosticToolOwner.#listeners.add(listener);
    return () => DiagnosticToolOwner.#listeners.delete(listener);
  }

  invoke(input: InvokeDiagnosticToolInput): Promise<unknown> {
    return this.#generation.invoke(input);
  }

  replaceRuntimeGeneration(): void {
    if (this.#disposed || DiagnosticToolOwner.#active !== this) return;
    const predecessor = this.#generation;
    this.#generation = new DiagnosticToolGeneration(this.#gateway);
    predecessor.retire();
    DiagnosticToolOwner.#advanceMaterialGeneration();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation.retire();
    if (DiagnosticToolOwner.#active === this) {
      DiagnosticToolOwner.#active = null;
      DiagnosticToolOwner.#advanceMaterialGeneration();
    }
  }

  static #advanceMaterialGeneration(): void {
    DiagnosticToolOwner.#materialGeneration += 1;
    for (const listener of DiagnosticToolOwner.#listeners) listener();
  }
}

export function invokeDiagnosticTool(input: InvokeDiagnosticToolInput): Promise<unknown> {
  return DiagnosticToolOwner.current().invoke(input);
}

export function diagnosticToolInvocationWasRetired(error: unknown): boolean {
  return error instanceof DiagnosticToolGenerationRetiredError;
}

export function diagnosticToolMaterialGeneration(): number {
  return DiagnosticToolOwner.materialGeneration();
}

export function subscribeDiagnosticToolMaterialGeneration(listener: () => void): () => void {
  return DiagnosticToolOwner.subscribeMaterialGeneration(listener);
}

export function formatDiagnosticToolResult(value: unknown): string {
  const encoded = JSON.stringify(value, null, 2);
  return encoded === undefined ? String(value) : encoded;
}
