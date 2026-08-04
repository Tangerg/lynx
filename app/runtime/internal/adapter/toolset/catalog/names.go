// Package catalog defines the stable model-facing identities of Runtime's
// built-in tools. Runtime-owned definitions and cross-cutting consumers use
// these names; parity tests bind SDK-owned definitions to the same vocabulary.
package catalog

const (
	ApplyPatch          = "apply_patch"
	AskUser             = "ask_user"
	CreateGoal          = "create_goal"
	CreateSchedule      = "create_schedule"
	DeleteSchedule      = "delete_schedule"
	DelegateTask        = "delegate_task"
	EnterPlanMode       = "enter_plan_mode"
	ExitPlanMode        = "exit_plan_mode"
	GetGoal             = "get_goal"
	Glob                = "glob"
	Grep                = "grep"
	HTTPRequest         = "http_request"
	ListSchedules       = "list_schedules"
	ListSkills          = "list_skills"
	LoadSkill           = "load_skill"
	LSP                 = "lsp"
	ProposeSkill        = "propose_skill"
	Read                = "read"
	ReadShellOutput     = "read_shell_output"
	ReadSkillResource   = "read_skill_resource"
	ReadToolResult      = "read_tool_result"
	ReportGoalOutcome   = "report_goal_outcome"
	SearchConversations = "search_conversations"
	SearchMemory        = "search_memory"
	SearchTools         = "search_tools"
	SetPlan             = "set_plan"
	Shell               = "shell"
	StopShell           = "stop_shell"
	WebFetch            = "web_fetch"
	WebSearch           = "web_search"
)
