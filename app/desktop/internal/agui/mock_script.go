package agui

// Static script content — mirrors frontend/src/protocol/agui/mockScript.ts so
// the demo conversation looks identical whether the mock runs in JS or Go.

var activityLines = []string{
	"pnpm typecheck · src/api/billing.ts",
	"pnpm typecheck · src/components/LoginForm.tsx",
	"pnpm typecheck · waiting for tsc watcher",
	"Reading src/api/billing.ts:142",
	"Scanning result of pnpm test --filter=auth",
}

const proposedCode = `export type Ok<T>  = { readonly ok: true;  readonly value: T };
export type Err<E> = { readonly ok: false; readonly error: E };
export type Result<T, E> = Ok<T> | Err<E>;

export const ok  = <T>(value: T): Ok<T>  => ({ ok: true,  value });
export const err = <E>(error: E): Err<E> => ({ ok: false, error });

export const map = <T, U, E>(r: Result<T, E>, f: (t: T) => U): Result<U, E> =>
  r.ok ? ok(f(r.value)) : r;

export const unwrapOr = <T, E>(r: Result<T, E>, fallback: T): T =>
  r.ok ? r.value : fallback;`

type planItem struct {
	ID     int    `json:"id"`
	PID    string `json:"pid"`
	Status string `json:"status"`
	Text   string `json:"text"`
}

var planItems = []planItem{
	{1, "T-001", "done", "Read src/api/auth.ts and identify error-throwing call sites"},
	{2, "T-002", "done", "Grep callers of login(), refresh(), and verifyToken() across the repo"},
	{3, "T-003", "done", "Introduce Result<T,E> type in src/lib/result.ts with helpers ok() and err()"},
	{4, "T-004", "done", "Refactor auth.ts to return Result instead of throwing"},
	{5, "T-005", "doing", "Update 7 call sites to handle the new return shape"},
	{6, "T-006", "todo", "Run pnpm test and pnpm typecheck"},
	{7, "T-007", "todo", "Update CHANGELOG.md and open a draft PR"},
}

type searchResult struct {
	Domain  string `json:"domain"`
	Title   string `json:"title"`
	Time    string `json:"time"`
	Snippet string `json:"snippet"`
}

var searchResults = []searchResult{
	{"matklad.github.io", "Result vs. Exceptions — why TypeScript adopted both", "2024",
		"The Result pattern shines for recoverable errors at boundaries; throwing remains idiomatic for programmer bugs."},
	{"effect.website", "Effect — Building blocks for typed errors", "2025",
		"Effect's tagged Result and Either generalize the pattern, but plain Result<T, E> is enough for most service-level code."},
	{"github.com/supermacro/neverthrow", "neverthrow — Type-safe errors for TS", "★ 5.2k",
		"Lightweight Result type with combinators (map, andThen, mapErr). The most popular pure-TS implementation."},
}

type toolSpec struct {
	ID         string
	Fn         string
	Args       string
	DurationMs int
	Added      *int
	Removed    *int
	Hits       *int
	Lines      *int
}

var toolScript = []toolSpec{
	{"t1", "read_file", "src/api/auth.ts", 12, nil, nil, nil, IntPtr(247)},
	{"t2", "grep", `"login\(|refresh\(|verifyToken\("`, 34, nil, nil, IntPtr(14), nil},
	{"tw", "web_search", "Result<T,E> type pattern TypeScript best practices", 1200, nil, nil, IntPtr(3), nil},
	{"t3", "write_file", "src/lib/result.ts", 8, IntPtr(38), IntPtr(0), nil, nil},
	{"t4", "edit_file", "src/api/auth.ts", 11, IntPtr(47), IntPtr(31), nil, nil},
	{"t5", "bash", "pnpm typecheck", 2400, nil, nil, nil, nil},
	{"t6", "edit_file", "src/api/billing.ts", 9, IntPtr(3), IntPtr(3), nil, nil},
}

const (
	userPrompt = "Refactor `src/api/auth.ts` to return `Result<T, E>` instead of throwing. " +
		"Update all call sites and make sure typecheck passes. Don't change the public auth flow."

	introText = "I'll handle this in 7 steps. Reading the file first to see the current shape, " +
		"then I'll introduce a `Result` type and refactor outward."

	postPlanText = "Pulling in the file and finding everywhere these functions are called."

	postGrepText = "Found **14 call sites across 7 files**. Before writing, let me check how teams " +
		"typically shape `Result<T,E>` in TypeScript — I want to keep this idiomatic."

	postSearchText = "Going with a minimal **discriminated-union** flavor — same shape as **neverthrow** " +
		"but without the dependency. Here's what I'll add to `src/lib/result.ts`:"

	postWriteText = "Now refactoring `auth.ts`. Three methods change shape: `login`, `refresh`, " +
		"`verifyToken`. None of the call signatures change — just the return type."

	postEditText = "Refactor applied — **+47 / −31**. Running typecheck to find any callers I haven't migrated:"

	postTypecheckText = "Got one error in `billing.ts:142` — a caller still expects `Session` directly. Fixing:"
)

// Custom event names — must match the constants in customEvents.ts on the JS side.
const (
	customPlan           = "lyra.plan"
	customPlanBlock      = "lyra.plan-block"
	customCodeProposal   = "lyra.code-proposal"
	customSearchResults  = "lyra.search-results"
	customApproval       = "lyra.approval"
	customTelemetry      = "lyra.telemetry"
)
