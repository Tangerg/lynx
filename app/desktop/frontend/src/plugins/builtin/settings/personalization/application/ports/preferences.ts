export type MessageStyle = "bubble" | "plain";
export type StreamReveal = "smooth" | "typewriter";

export interface PersonalizationPreferencesPort {
  useMessageStyle(): MessageStyle;
  useSetMessageStyle(): (style: MessageStyle) => void;
  useCompletionSound(): boolean;
  useSetCompletionSound(): (on: boolean) => void;
  useStreamReveal(): StreamReveal;
  useSetStreamReveal(): (mode: StreamReveal) => void;
}

let port: PersonalizationPreferencesPort | null = null;

export function configurePersonalizationPreferencesPort(
  next: PersonalizationPreferencesPort,
): void {
  port = next;
}

export function personalizationPreferences(): PersonalizationPreferencesPort {
  if (!port) throw new Error("Personalization preferences port is not configured");
  return port;
}
