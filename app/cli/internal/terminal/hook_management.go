package terminal

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/hookpolicy"
)

func (a *app) ShowHooks() {
	if a.hooks == nil {
		a.message("this runtime composition has no hook service")
		return
	}
	a.executeRuntimeReaderQuery(a.hooksReaderQuery())
}

func (a *app) hooksReaderQuery() runtimeReaderQuery {
	workspace := a.session.Workspace.Path
	return runtimeReaderQuery{
		status: "loading lifecycle hooks", mode: runtimeReaderHooks,
		read: func(ctx context.Context) (readerDocument, error) {
			catalog, err := a.hooks.Catalog(ctx, workspace)
			if err != nil {
				return readerDocument{}, err
			}
			return hooksDocument(workspace, catalog), nil
		},
	}
}

func hooksDocument(workspace string, catalog hookpolicy.Catalog) readerDocument {
	detail := fmt.Sprintf("%d hooks · project trust %t", len(catalog.Hooks), catalog.ProjectTrusted)
	if catalog.ProjectRoot != "" {
		detail += " · " + catalog.ProjectRoot
	}
	if len(catalog.Hooks) == 0 {
		return paragraphDocument("Lifecycle hooks", detail, []string{"No global or project hooks are configured for " + workspace + "."})
	}
	sections := make([]ToolSection, 0, len(catalog.Hooks)*2)
	for _, hook := range catalog.Hooks {
		state := "inactive"
		if hook.Active {
			state = "active"
		}
		metadata := []string{"scope    " + string(hook.Scope), "state    " + state, "source   " + hook.Source}
		if hook.Matcher != "" {
			metadata = append(metadata, "matcher  "+hook.Matcher)
		}
		if hook.TimeoutMillis > 0 {
			metadata = append(metadata, "timeout  "+strconv.Itoa(hook.TimeoutMillis)+"ms")
		}
		actionTitle, action := "Injected context", hook.Inject
		if hook.Command != "" {
			actionTitle, action = "Shell command", hook.Command
		}
		sections = append(sections,
			ToolSection{Title: string(hook.Event) + " · " + state, Style: toolSectionCode, Language: "text", Text: strings.Join(metadata, "\n")},
			ToolSection{Title: actionTitle, Style: toolSectionCode, Language: "text", Text: action},
		)
	}
	return readerDocument{Title: "Lifecycle hooks", Detail: detail, Sections: sections}
}

func (a *app) PrepareHookTrust(trusted bool) error {
	if a.hooks == nil {
		return errors.New("this runtime composition has no hook service")
	}
	workspace := a.session.Workspace.Path
	a.status.note("loading project hook trust")
	if !a.runOperation(hookOperation, false,
		func(ctx context.Context) (hookpolicy.Catalog, error) { return a.hooks.Catalog(ctx, workspace) },
		func(catalog hookpolicy.Catalog, err error) {
			if err != nil {
				a.message("load project hooks failed: " + err.Error())
				return
			}
			if catalog.ProjectRoot == "" {
				a.message("this workspace has no project root to trust")
				return
			}
			if catalog.ProjectTrusted == trusted {
				state := "revoked"
				if trusted {
					state = "trusted"
				}
				a.message("project hooks are already " + state + " · " + catalog.ProjectRoot)
				return
			}
			title, question, action := "Revoke project hooks", "Disable project hooks from "+catalog.ProjectRoot+"?", "Revoke trust"
			if trusted {
				title, question, action = "Trust project hooks", "Allow reviewed project hooks from "+catalog.ProjectRoot+" to execute?", "Trust hooks"
			}
			a.confirmAction(title, question, action, func() { a.setHookTrust(workspace, catalog.ProjectRoot, trusted) })
		},
	) {
		return errors.New("another hook operation is running")
	}
	return nil
}

func (a *app) setHookTrust(workspace, projectRoot string, trusted bool) {
	presentation := a.sessionContext
	a.status.note("updating project hook trust")
	if !a.runAdmissionMutation(hookOperation, false,
		func(ctx context.Context) (hookpolicy.Catalog, error) {
			if err := a.hooks.SetProjectTrust(ctx, projectRoot, trusted); err != nil {
				return hookpolicy.Catalog{}, err
			}
			catalog, err := a.hooks.Catalog(ctx, workspace)
			if err != nil {
				return hookpolicy.Catalog{}, err
			}
			if err := catalog.ValidateTrustAcknowledgement(projectRoot, trusted); err != nil {
				return hookpolicy.Catalog{}, fmt.Errorf("verify project hook trust: %w", err)
			}
			return catalog, nil
		},
		func(catalog hookpolicy.Catalog, err error) {
			if err != nil {
				a.message("update project hook trust failed: " + err.Error())
				return
			}
			if a.sessionContext == presentation {
				a.setRuntimeReader(runtimeReaderHooks)
				a.openReaderDocument(hooksDocument(workspace, catalog))
			}
			a.status.note("project hook trust updated")
		},
	) {
		a.message("another hook operation is running")
	}
}
