package agent

type ApprovalMode string

const (
	ApprovalModeSafe     ApprovalMode = "safe"
	ApprovalModeBalanced ApprovalMode = "balanced"
	ApprovalModeYolo     ApprovalMode = "yolo"
)

type ApprovalRule struct {
	ID       string
	Scope    RememberScope
	Tool     string
	Subject  string
	Dir      string
	Decision ApprovalRuleDecision
}

type ApprovalRuleDecision string

const (
	ApprovalRuleAllow ApprovalRuleDecision = "allow"
	ApprovalRuleDeny  ApprovalRuleDecision = "deny"
)
