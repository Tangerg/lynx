package terminal

type workbenchConcern uint8

const (
	workbenchResumeOutbox workbenchConcern = iota
	workbenchRunOutbox
	workbenchDraft
	workbenchHistory
	workbenchConcernCount
)

type workbenchHealth struct {
	problems [workbenchConcernCount]string
}

func (health *workbenchHealth) update(concern workbenchConcern, err error) bool {
	problem := ""
	if err != nil {
		problem = "workbench: " + err.Error()
	}
	if health.problems[concern] == problem {
		return false
	}
	health.problems[concern] = problem
	return true
}

func (health workbenchHealth) problem() string {
	for _, problem := range health.problems {
		if problem != "" {
			return problem
		}
	}
	return ""
}

func (a *app) reportWorkbenchIssue(concern workbenchConcern, err error) {
	if a.workbenchHealth.update(concern, err) {
		a.status.setProblem(a.workbenchHealth.problem())
	}
}
