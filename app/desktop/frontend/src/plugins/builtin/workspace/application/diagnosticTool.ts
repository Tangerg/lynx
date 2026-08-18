import { z } from "zod";
import { createPublicationSlot } from "@/lib/publicationSlot";
import { RetirableTaskCohort } from "@/lib/taskQueue";
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
  readonly #cohort = new RetirableTaskCohort(this.#retiredError);
  readonly #tails = new Map<string, Promise<void>>();

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
    this.#cohort.retire();
    this.#tails.clear();
  }

  #settle<T>(operation: Promise<T>): Promise<T> {
    return this.#cohort.settle(operation);
  }

  #assertCurrent(): void {
    this.#cohort.assertCurrent();
  }
}

/** Owns direct Tool invocations for one exact Plugin Host and Runtime generation. */
export class DiagnosticToolOwner {
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
    const owner = new DiagnosticToolOwner(gateway);
    diagnosticToolPublication.publish(owner, (predecessor) => predecessor.dispose());
    DiagnosticToolOwner.#advanceMaterialGeneration();
    return owner;
  }

  static current(): DiagnosticToolOwner {
    const owner = diagnosticToolPublication.current();
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
    if (this.#disposed || !diagnosticToolPublication.owns(this)) return;
    const predecessor = this.#generation;
    this.#generation = new DiagnosticToolGeneration(this.#gateway);
    predecessor.retire();
    DiagnosticToolOwner.#advanceMaterialGeneration();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation.retire();
    if (diagnosticToolPublication.withdraw(this)) {
      DiagnosticToolOwner.#advanceMaterialGeneration();
    }
  }

  static #advanceMaterialGeneration(): void {
    DiagnosticToolOwner.#materialGeneration += 1;
    for (const listener of DiagnosticToolOwner.#listeners) listener();
  }
}

const diagnosticToolPublication = createPublicationSlot<DiagnosticToolOwner>();

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
