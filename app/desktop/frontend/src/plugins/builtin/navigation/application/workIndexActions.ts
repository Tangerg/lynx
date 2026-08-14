import { useMemo } from "react";
import { t } from "@/lib/i18n";
import {
  selectAgentSession,
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
import { workingDirectoryPicker } from "./ports/workingDirectoryPicker";

export interface WorkIndexActions {
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
  if (pendingFolderSession) return pendingFolderSession;
  const pending = (async () => {
    const cwd = await workingDirectoryPicker().choose();
    if (!cwd) return;
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
  const remove = useDeleteSession();
  const fork = useForkSession();
  const rename = useRenameSession();
  const toggleFavorite = useToggleFavorite();

  return useMemo(
    () => ({
      // A fresh session may already be on screen, in which case create() hands
      // that one back and nothing moves — so the caret is the acknowledgement.
      createSession: () => {
        void create().then(() => focusComposer());
      },
      chooseSessionFolder: () => {
        void createSessionInChosenFolder(create);
      },
      startSessionInFolder: (cwd) => {
        void create({ cwd }).then(() => focusComposer());
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
    [create, fork, remove, rename, toggleFavorite],
  );
}
