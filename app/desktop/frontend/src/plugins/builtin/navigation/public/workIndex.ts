export {
  contributeWorkIndexItem,
  type WorkIndexItemScope,
  type WorkIndexItemSpec,
  type WorkIndexItemVariant,
  useWorkIndexItems,
} from "../application/workIndexContributions";
export { useWorkIndexActions, type WorkIndexActions } from "../application/workIndexActions";
export { useRecentWorkSessions, useWorkIndex } from "../application/useWorkIndex";
export type {
  WorkGroup,
  WorkIndex,
  WorkProject,
  WorkSession,
  WorkSessionAttention,
} from "../domain/workIndex";
