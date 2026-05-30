// CUSTOM event name discriminators. Payload shapes live in
// `schemas.ts` as Zod schemas (one source of truth: TS type +
// runtime validation), referenced by name through this constant.

export const CUSTOM = {
  PLAN: "lyra.plan",
  PLAN_BLOCK: "lyra.plan-block",
  CODE_PROPOSAL: "lyra.code-proposal",
  SEARCH_RESULTS: "lyra.search-results",
  APPROVAL: "lyra.approval",
  APPROVAL_RESULT: "lyra.approval-result",
  QUESTION: "lyra.question",
  QUESTION_RESULT: "lyra.question-result",
  TELEMETRY: "lyra.telemetry",
} as const;
