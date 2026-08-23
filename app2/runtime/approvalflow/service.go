// Package approvalflow owns approval-policy use cases and their Runtime event
// boundary. It is independent from one-shot Interrupt persistence and schedule
// execution because those concepts have different lifecycles.
package approvalflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/approvalpolicy"
	"github.com/Tangerg/lynx/app2/runtime/domain/session"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type Store interface {
	GetApprovalMode(context.Context) (approvalpolicy.Mode, error)
	SetApprovalMode(context.Context, approvalpolicy.Mode) (bool, error)
	ListVisibleApprovalRules(context.Context, string, string) ([]approvalpolicy.Rule, error)
	PutApprovalRule(context.Context, approvalpolicy.Rule) (bool, error)
	DeleteApprovalRule(context.Context, string) (bool, error)
}

type Sessions interface {
	GetSession(context.Context, session.ID) (session.Session, error)
}

type Events interface {
	Publish(protocol.RuntimeEvent)
}

type Config struct {
	Store    Store
	Sessions Sessions
	Events   Events
	Clock    func() time.Time
}

type Service struct {
	store    Store
	sessions Sessions
	events   Events
	now      func() time.Time
}

func New(config Config) (*Service, error) {
	if config.Store == nil || config.Sessions == nil || config.Events == nil {
		return nil, errors.New("approvalflow: store, sessions, and events are required")
	}
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Service{store: config.Store, sessions: config.Sessions, events: config.Events, now: clock}, nil
}

func (service *Service) Mode(ctx context.Context) (approvalpolicy.Mode, error) {
	return service.store.GetApprovalMode(ctx)
}

func (service *Service) GetMode(ctx context.Context) (*protocol.ApprovalModeResult, error) {
	mode, err := service.Mode(ctx)
	if err != nil {
		return nil, err
	}
	return &protocol.ApprovalModeResult{Mode: protocol.ApprovalMode(mode)}, nil
}

func (service *Service) SetMode(
	ctx context.Context,
	request protocol.SetApprovalModeRequest,
) (*protocol.ApprovalModeResult, error) {
	mode := approvalpolicy.Mode(request.Mode)
	if !mode.Valid() {
		return nil, fmt.Errorf("%w: approval mode is invalid", protocol.ErrInvalidParams)
	}
	changed, err := service.store.SetApprovalMode(ctx, mode)
	if err != nil {
		return nil, err
	}
	if changed {
		service.publish()
	}
	return &protocol.ApprovalModeResult{Mode: request.Mode}, nil
}

func (service *Service) ListRules(
	ctx context.Context,
	request protocol.ListApprovalRulesRequest,
) (*protocol.ListApprovalRulesResult, error) {
	projectDir := ""
	value, err := service.sessions.GetSession(ctx, session.ID(request.SessionID))
	if err == nil {
		projectDir = value.Workspace().Path()
	} else if !errors.Is(err, session.ErrNotFound) {
		return nil, err
	}
	rules, err := service.store.ListVisibleApprovalRules(ctx, request.SessionID, projectDir)
	if err != nil {
		return nil, err
	}
	result := make([]protocol.ApprovalRule, len(rules))
	for index, rule := range rules {
		result[index] = presentRule(rule)
	}
	return &protocol.ListApprovalRulesResult{Rules: result}, nil
}

func (service *Service) ForgetRule(
	ctx context.Context,
	request protocol.ForgetApprovalRuleRequest,
) error {
	changed, err := service.store.DeleteApprovalRule(ctx, request.ID)
	if err != nil {
		return err
	}
	if changed {
		service.publish()
	}
	return nil
}

func (service *Service) Decide(
	ctx context.Context,
	query approvalpolicy.Query,
) (approvalpolicy.Decision, bool, error) {
	rules, err := service.store.ListVisibleApprovalRules(ctx, query.SessionID, query.ProjectDir)
	if err != nil {
		return "", false, err
	}
	return approvalpolicy.Decide(rules, query)
}

func (service *Service) Remember(
	ctx context.Context,
	command approvalpolicy.Remember,
) error {
	command.Now = service.now()
	rule, err := approvalpolicy.NewRemembered(command)
	if err != nil {
		return err
	}
	changed, err := service.store.PutApprovalRule(ctx, rule)
	if err != nil {
		return err
	}
	if changed {
		service.publish()
	}
	return nil
}

func (service *Service) publish() {
	service.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeApprovalsChanged})
}

func presentRule(rule approvalpolicy.Rule) protocol.ApprovalRule {
	value := protocol.ApprovalRule{
		ID: rule.ID(), Scope: protocol.ApprovalRuleScope(rule.Scope()),
		Tool: rule.Tool(), Subject: rule.Subject(),
		Decision: protocol.ApprovalRuleDecision(rule.Decision()),
	}
	if rule.Scope() == approvalpolicy.ScopeProject {
		value.Dir = rule.ScopeKey()
	}
	return value
}
