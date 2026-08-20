import { useMemo } from "react";
import { t } from "@/lib/i18n";
import {
  selectAgentSession,
  useActiveSessionId,
  useActiveSessionWorkspace,
  useCreateSession,
  useDeleteSession,
  useForkSession,
  useRenameSession,
  useToggleFavorite,
} from "@/plugins/builtin/agent/public/session";
import { focusComposer } from "@/plugins/builtin/chat/composer/public/focus";
import { showWorkspaceDock } from "@/plugins/builtin/workspace/public/navigation";
import { openSettingsView } from "@/plugins/builtin/workspace/public/deeplinks";
import { notifyError } from "@/plugins/sdk";
import {
  runtimeCommandsAvailable,
  useRuntimeCommandsAvailable,
} from "@/plugins/builtin/runtime/public/serviceStatus";
import { workingDirectoryPicker } from "./ports/workingDirectoryPicker";

export interface WorkIndexActions {
  canCreateSession: boolean;
  canCreateSessionInFolder: boolean;
  createSession: () => void;
  chooseSessionFolder: () => void;
  startSessionInFolder: (cwd: string) => void;
  selectSession: (id: string) => void;
  renameSession: (id: string, expectedRevision: number, title: string) => void;
  forkSession: (id: string) => void;
  deleteSession: (id: string) => void;
  toggleFavorite: (id: string, expectedRevision: number, favorite: boolean) => void;
  openContextDock: () => void;
  openSettings: () => void;
}

let pendingFolderSession: Promise<void> | null = null;

function reportDirectorySelectionError(error: unknown): void {
  notifyError(t("session.error.chooseWorkingDirectory"), {
    description: error instanceof Error ? error.message : undefined,
    source: "session",
  });
}

function createSessionInChosenFolder(create: ReturnType<typeof useCreateSession>): Promise<void> {
  if (!runtimeCommandsAvailable()) return Promise.resolve();
  if (pendingFolderSession) return pendingFolderSession;
  const pending = (async () => {
    const cwd = await workingDirectoryPicker().choose();
    if (!cwd || !runtimeCommandsAvailable()) return;
    if (await create({ cwd })) focusComposer();
  })()
    .catch(reportDirectorySelectionError)
    .finally(() => {
      if (pendingFolderSession === pending) pendingFolderSession = null;
    });
  pendingFolderSession = pending;
  return pending;
}

export function useWorkIndexActions(): WorkIndexActions {
  const create = useCreateSession();
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const activeSessionId = useActiveSessionId();
  const activeWorkspace = useActiveSessionWorkspace();
  const activeWorkspaceStatus = activeWorkspace.status;
  const activeCwd = activeWorkspace.status === "ready" ? activeWorkspace.cwd : undefined;
  const remove = useDeleteSession();
  const fork = useForkSession();
  const rename = useRenameSession();
  const toggleFavorite = useToggleFavorite();

  return useMemo(
    () => ({
      canCreateSession:
        runtimeAvailable &&
        (!activeSessionId ||
          (activeWorkspaceStatus === "ready" && Boolean(activeCwd && activeCwd.trim()))),
      canCreateSessionInFolder: runtimeAvailable,
      // Codex's top-level New action continues in the project that owns the
      // active Session. Omitting cwd here delegated that decision to the
      // Runtime process default, so the same click could silently jump projects.
      // Bind the exact rendered workspace before the async create; while the
      // active summary is resolving, the action is disabled rather than
      // inventing a default owner.
      createSession: () => {
        if (!runtimeCommandsAvailable()) return;
        if (!activeSessionId) {
          focusComposer();
          return;
        }
        if (activeWorkspaceStatus !== "ready" || !activeCwd?.trim()) return;
        void create({ cwd: activeCwd, reuseFreshDraft: true }).then((sessionId) => {
          if (sessionId) focusComposer();
        });
      },
      chooseSessionFolder: () => {
        if (!runtimeCommandsAvailable()) return;
        void createSessionInChosenFolder(create);
      },
      startSessionInFolder: (cwd) => {
        if (!runtimeCommandsAvailable()) return;
        void create({ cwd }).then((sessionId) => {
          if (sessionId) focusComposer();
        });
      },
      selectSession: selectAgentSession,
      renameSession: (id, expectedRevision, title) => {
        void rename(id, expectedRevision, title);
      },
      forkSession: (id) => {
        void fork(id);
      },
      deleteSession: (id) => {
        void remove(id);
      },
      toggleFavorite: (id, expectedRevision, favorite) => {
        void toggleFavorite(id, expectedRevision, favorite);
      },
      openContextDock: showWorkspaceDock,
      openSettings: () => {
        openSettingsView();
      },
    }),
    [
      activeCwd,
      activeSessionId,
      activeWorkspaceStatus,
      create,
      fork,
      remove,
      rename,
      runtimeAvailable,
      toggleFavorite,
    ],
  );
}
