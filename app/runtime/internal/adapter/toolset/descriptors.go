package toolset

import (
	"iter"
	"reflect"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolname"
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
			{toolname.Read, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading file"}},
			{toolname.Glob, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Finding files", result: searchResultContract()}},
			{toolname.Grep, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Searching", result: searchResultContract()}},
			{toolname.LSP, builtInDescriptor{safety: tool.SafetyClassSafe, activity: lspActivity}},
			{toolname.ReadShellOutput, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading command output"}},
			{toolname.ListSchedules, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Listing schedules"}},
			{toolname.ListSkills, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Listing Skills"}},
			{toolname.LoadSkill, builtInDescriptor{safety: tool.SafetyClassSafe, activity: loadSkillActivity}},
			{toolname.ReadSkillResource, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading a Skill resource"}},
			{toolname.SearchMemory, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Searching project memory"}},
			{toolname.SearchConversations, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Searching earlier conversations"}},
			{toolname.SearchTools, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Loading additional tools"}},
			{toolname.AskUser, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Waiting for your answer"}},
			{toolname.EnterPlanMode, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Entering Plan mode"}},
			{toolname.ExitPlanMode, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Requesting Plan approval"}},
			{toolname.SetPlan, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Updating the Plan", outcome: planOutcomeProjection}},
			{toolname.ReadToolResult, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading omitted tool output"}},
			{toolname.DelegateTask, builtInDescriptor{safety: tool.SafetyClassSafe, activity: delegationActivity, orchestration: true}},
			{toolname.CreateGoal, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Starting an autonomous Goal"}},
			{toolname.GetGoal, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Inspecting the autonomous Goal"}},
			{toolname.ReportGoalOutcome, builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reporting a Goal outcome"}},
			{toolname.ProposeSkill, builtInDescriptor{safety: tool.SafetyClassSafe, activity: proposeSkillActivity}},
			{toolname.ApplyPatch, builtInDescriptor{safety: tool.SafetyClassWrite, activityText: "Applying a patch", result: patchResultContract()}},
			{toolname.CreateSchedule, builtInDescriptor{safety: tool.SafetyClassWrite, activity: createScheduleActivity}},
			{toolname.DeleteSchedule, builtInDescriptor{safety: tool.SafetyClassWrite, activityText: "Deleting a schedule"}},
			{toolname.Shell, builtInDescriptor{safety: tool.SafetyClassExec, activity: shellActivity, result: commandResultContract()}},
			{toolname.StopShell, builtInDescriptor{safety: tool.SafetyClassExec, activityText: "Stopping command"}},
			{toolname.WebFetch, builtInDescriptor{safety: tool.SafetyClassNetwork, activityText: "Fetching a page"}},
			{toolname.WebSearch, builtInDescriptor{safety: tool.SafetyClassNetwork, activityText: "Searching the web", result: webSearchResultContract()}},
			{toolname.HTTPRequest, builtInDescriptor{safety: tool.SafetyClassNetwork, activity: httpActivity}},
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
