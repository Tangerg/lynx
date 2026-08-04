package toolset

import (
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/catalog"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

type activityProjection func(tool.Arguments) string
type resultProjection func(tool.Arguments, tool.Result) (tool.Result, string)

type outcomeProjection uint8

const planOutcomeProjection outcomeProjection = 1

// builtInDescriptor is the single behavioral catalog for built-in identities.
// Tool constructors own model descriptions and schemas; this catalog owns the
// cross-cutting policy and client projection attached to those definitions.
type builtInDescriptor struct {
	safety        tool.SafetyClass
	activityText  string
	activity      activityProjection
	presentation  resultProjection
	orchestration bool
	outcome       outcomeProjection
}

func descriptorFor(name string) (builtInDescriptor, bool) {
	switch name {
	case catalog.Read:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading file"}, true
	case catalog.Glob:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Finding files", presentation: presentSearch}, true
	case catalog.Grep:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Searching", presentation: presentSearch}, true
	case catalog.LSP:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activity: lspActivity}, true
	case catalog.ReadShellOutput:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading command output"}, true
	case catalog.ListSchedules:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Listing schedules"}, true
	case catalog.ListSkills:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Listing Skills"}, true
	case catalog.LoadSkill:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activity: loadSkillActivity}, true
	case catalog.ReadSkillResource:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading a Skill resource"}, true
	case catalog.SearchMemory:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Searching project memory"}, true
	case catalog.SearchConversations:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Searching earlier conversations"}, true
	case catalog.SearchTools:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Loading additional tools"}, true
	case catalog.AskUser:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Waiting for your answer"}, true
	case catalog.EnterPlanMode:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Entering Plan mode"}, true
	case catalog.ExitPlanMode:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Requesting Plan approval"}, true
	case catalog.SetPlan:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Updating the Plan", outcome: planOutcomeProjection}, true
	case catalog.ReadToolResult:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading omitted tool output"}, true
	case catalog.DelegateTask:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activity: delegationActivity, orchestration: true}, true
	case catalog.CreateGoal:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Starting an autonomous Goal"}, true
	case catalog.GetGoal:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Inspecting the autonomous Goal"}, true
	case catalog.ReportGoalOutcome:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reporting a Goal outcome"}, true
	case catalog.ProposeSkill:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activity: proposeSkillActivity}, true
	case catalog.ApplyPatch:
		return builtInDescriptor{safety: tool.SafetyClassWrite, activityText: "Applying a patch", presentation: presentApplyPatch}, true
	case catalog.CreateSchedule:
		return builtInDescriptor{safety: tool.SafetyClassWrite, activity: createScheduleActivity}, true
	case catalog.DeleteSchedule:
		return builtInDescriptor{safety: tool.SafetyClassWrite, activityText: "Deleting a schedule"}, true
	case catalog.Shell:
		return builtInDescriptor{safety: tool.SafetyClassExec, activity: shellActivity, presentation: presentCommand}, true
	case catalog.StopShell:
		return builtInDescriptor{safety: tool.SafetyClassExec, activityText: "Stopping command"}, true
	case catalog.WebFetch:
		return builtInDescriptor{safety: tool.SafetyClassNetwork, activityText: "Fetching a page"}, true
	case catalog.WebSearch:
		return builtInDescriptor{safety: tool.SafetyClassNetwork, activityText: "Searching the web", presentation: presentWebSearch}, true
	case catalog.HTTPRequest:
		return builtInDescriptor{safety: tool.SafetyClassNetwork, activity: httpActivity}, true
	default:
		return builtInDescriptor{}, false
	}
}
