/**
 * The model-picker projection kept by Desktop. Runtime remains authoritative
 * for capability discovery and execution admission; this value owns only the
 * immutable client-side behavior shared by the picker, composer and context
 * gauge.
 */
export class SelectableModel {
  readonly id: string;
  readonly provider: string;
  readonly label: string;
  readonly contextWindow?: number;
  readonly maxInputTokens?: number;
  readonly maxOutputTokens?: number;
  readonly knowledgeCutoff?: string;
  readonly deprecated: boolean;
  readonly reasoning: boolean;
  readonly reasoningLevels: readonly string[];
  readonly reasoningDefaultLevel?: string;
  readonly inputModalities: readonly string[];
  readonly outputModalities: readonly string[];
  readonly toolUse: boolean;
  readonly structuredOutput: boolean;

  constructor(value: {
    id: string;
    provider: string;
    label: string;
    contextWindow?: number;
    maxInputTokens?: number;
    maxOutputTokens?: number;
    knowledgeCutoff?: string;
    deprecated?: boolean;
    reasoning?: boolean;
    reasoningLevels?: readonly string[];
    reasoningDefaultLevel?: string;
    inputModalities?: readonly string[];
    outputModalities?: readonly string[];
    toolUse?: boolean;
    structuredOutput?: boolean;
  }) {
    this.id = value.id;
    this.provider = value.provider;
    this.label = value.label;
    this.contextWindow = value.contextWindow;
    this.maxInputTokens = value.maxInputTokens;
    this.maxOutputTokens = value.maxOutputTokens;
    this.knowledgeCutoff = value.knowledgeCutoff;
    this.deprecated = value.deprecated ?? false;
    this.reasoning = value.reasoning ?? false;
    this.reasoningLevels = Object.freeze([...(value.reasoningLevels ?? [])]);
    this.reasoningDefaultLevel = value.reasoningDefaultLevel;
    this.inputModalities = Object.freeze([...(value.inputModalities ?? [])]);
    this.outputModalities = Object.freeze([...(value.outputModalities ?? [])]);
    this.toolUse = value.toolUse ?? false;
    this.structuredOutput = value.structuredOutput ?? false;
    Object.freeze(this);
  }

  acceptsInput(modality: string): boolean {
    return this.inputModalities.includes(modality);
  }

  acceptsReasoningLevel(level: string): boolean {
    return this.reasoning && this.reasoningLevels.includes(level);
  }

  reasoningLevelOrDefault(level?: string | null): string | undefined {
    if (!this.reasoning) return undefined;
    if (level && this.acceptsReasoningLevel(level)) return level;
    if (this.reasoningDefaultLevel && this.acceptsReasoningLevel(this.reasoningDefaultLevel)) {
      return this.reasoningDefaultLevel;
    }
    return this.reasoningLevels[0];
  }
}
