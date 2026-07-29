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
