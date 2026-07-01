export interface AgentTextInput {
  kind: "text";
  text: string;
}

export interface AgentImageInput {
  kind: "image";
  mime: string;
  data: string;
}

export type AgentInputPart = AgentTextInput | AgentImageInput;

export interface AgentInput {
  parts: AgentInputPart[];
}

export function agentTextInput(text: string): AgentInput {
  return text ? { parts: [{ kind: "text", text }] } : { parts: [] };
}

export function agentInputText(input: AgentInput): string {
  return input.parts
    .filter((part): part is AgentTextInput => part.kind === "text")
    .map((part) => part.text)
    .join("\n")
    .trim();
}
