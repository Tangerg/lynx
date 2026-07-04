import type { CommandSpec } from "@/plugins/sdk";

export interface WelcomeSuggestion {
  icon: string;
  labelKey: string;
  promptKey: string;
}

export interface ProviderSetupInfo {
  apiKeyMasked: string;
}

const HINT_COMMAND_IDS = ["command.open", "chat.new", "view.toggle-sidebar"];

export const WELCOME_SUGGESTIONS: WelcomeSuggestion[] = [
  {
    icon: "spark",
    labelKey: "welcome.suggest.refactor",
    promptKey: "welcome.suggest.refactor.prompt",
  },
  {
    icon: "search",
    labelKey: "welcome.suggest.search",
    promptKey: "welcome.suggest.search.prompt",
  },
  {
    icon: "branch",
    labelKey: "welcome.suggest.review",
    promptKey: "welcome.suggest.review.prompt",
  },
  {
    icon: "list",
    labelKey: "welcome.suggest.checklist",
    promptKey: "welcome.suggest.checklist.prompt",
  },
];

export function welcomeHintCommands(commands: CommandSpec[]): CommandSpec[] {
  return HINT_COMMAND_IDS.map((id) => commands.find((command) => command.id === id)).filter(
    (command): command is CommandSpec => command?.combo !== undefined,
  );
}

export function needsProviderSetup(providers: ProviderSetupInfo[] | undefined): boolean {
  return providers !== undefined && !providers.some((provider) => provider.apiKeyMasked !== "");
}
