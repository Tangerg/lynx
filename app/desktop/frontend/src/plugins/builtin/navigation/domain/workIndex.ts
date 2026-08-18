export type WorkSessionAttention = "running" | "waiting" | "none";

export interface WorkSession {
  id: string;
  revision: number;
  title: string;
  attention: WorkSessionAttention;
  favorite?: boolean;
  time: string;
}

export interface WorkProject {
  id: string;
  name: string;
  cwdMissing?: boolean;
}

export interface WorkGroup {
  project: WorkProject;
  sessions: WorkSession[];
}

/**
 * The index splits every session exactly once.
 *
 * A session belongs to a project when its directory is one the workspace knows;
 * everything else — scratch directories, sessions started before a folder was
 * picked — is recent work with no home yet. Two lists rather than one tree
 * because inventing a project from an arbitrary path gives scratch work a false home.
 */
export interface WorkIndexContent {
  groups: WorkGroup[];
  recents: WorkSession[];
}

export interface WorkIndex {
  /** Both absent until the first answer arrives — distinct from "known empty". */
  groups: WorkGroup[] | undefined;
  recents: WorkSession[] | undefined;
  activeSessionId: string;
  activeCwd: string | undefined;
  isLoading: boolean;
  isError: boolean;
}
