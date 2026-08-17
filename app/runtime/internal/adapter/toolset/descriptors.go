package toolset

import (
	"iter"
	"reflect"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

type activityProjection func(tool.Arguments) string
type resultProjection func(tool.Arguments, tool.Result) (tool.Result, string)

type outcomeProjection uint8

const planOutcomeProjection outcomeProjection = 1

type resultProjectionContract struct {
	project    resultProjection
	resultType reflect.Type
	enums      func() map[reflect.Type][]string
}

// builtInDescriptor is the single behavioral catalog for built-in identities.
// Tool constructors own model descriptions and schemas; this catalog owns the
// cross-cutting policy and client projection attached to those definitions.
type builtInDescriptor struct {
	safety        tool.SafetyClass
	activityText  string
	activity      activityProjection
	result        resultProjectionContract
	orchestration bool
	outcome       outcomeProjection
}

func descriptors() iter.Seq2[string, builtInDescriptor] {
	return func(yield func(string, builtInDescriptor) bool) {
		for _, entry := range []struct {
			name       string
			descriptor builtInDescriptor
		}{
			{tool.Read, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading file"}},
			{tool.Glob, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Finding files", result: searchResultContract()}},
			{tool.Grep, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Searching", result: searchResultContract()}},
			{tool.LSP, builtInDescriptor{safety: tool.SafetyClassSafe, activity: lspActivity}},
			{tool.ReadShellOutput, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading command output"}},
			{tool.ListSchedules, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Listing schedules"}},
			{tool.ListSkills, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Listing Skills"}},
			{tool.LoadSkill, builtInDescriptor{safety: tool.SafetyClassSafe, activity: loadSkillActivity}},
			{tool.ReadSkillResource, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading a Skill resource"}},
			{tool.SearchMemory, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Searching project memory"}},
			{tool.SearchConversations, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Searching earlier conversations"}},
			{tool.SearchTools, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Loading additional tools"}},
			{tool.AskUser, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Waiting for your answer"}},
			{tool.EnterPlanMode, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Entering Plan mode"}},
			{tool.ExitPlanMode, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Requesting Plan approval"}},
			{tool.SetPlan, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Updating the Plan", outcome: planOutcomeProjection}},
			{tool.ReadToolResult, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading omitted tool output"}},
			{tool.DelegateTask, builtInDescriptor{safety: tool.SafetyClassSafe, activity: delegationActivity, orchestration: true}},
			{tool.CreateGoal, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Starting an autonomous Goal"}},
			{tool.GetGoal, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Inspecting the autonomous Goal"}},
			{tool.ReportGoalOutcome, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reporting a Goal outcome"}},
			{tool.ProposeSkill, builtInDescriptor{safety: tool.SafetyClassSafe, activity: proposeSkillActivity}},
			{tool.ApplyPatch, builtInDescriptor{safety: tool.SafetyClassWrite, activityText: "Applying a patch", result: patchResultContract()}},
			{tool.CreateSchedule, builtInDescriptor{safety: tool.SafetyClassWrite, activity: createScheduleActivity}},
			{tool.DeleteSchedule, builtInDescriptor{safety: tool.SafetyClassWrite, activityText: "Deleting a schedule"}},
			{tool.Shell, builtInDescriptor{safety: tool.SafetyClassExec, activity: shellActivity, result: commandResultContract()}},
			{tool.StopShell, builtInDescriptor{safety: tool.SafetyClassExec, activityText: "Stopping command"}},
			{tool.WebFetch, builtInDescriptor{safety: tool.SafetyClassNetwork, activityText: "Fetching a page"}},
			{tool.WebSearch, builtInDescriptor{safety: tool.SafetyClassNetwork, activityText: "Searching the web", result: webSearchResultContract()}},
			{tool.HTTPRequest, builtInDescriptor{safety: tool.SafetyClassNetwork, activity: httpActivity}},
		} {
			if !yield(entry.name, entry.descriptor) {
				return
			}
		}
	}
}

func descriptorFor(name string) (builtInDescriptor, bool) {
	for candidate, descriptor := range descriptors() {
		if candidate == name {
			return descriptor, true
		}
	}
	return builtInDescriptor{}, false
}
