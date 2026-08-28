package agentexec

import (
	"fmt"
	"slices"
	"strings"

	corechat "github.com/Tangerg/scope/core/chat"
	coremetadata "github.com/Tangerg/scope/core/metadata"
)

const (
	contextProvenanceSchemaVersion uint16 = 1
	contextProvenanceMetadataKey          = "scope/context_provenance"
)

type contextSourceKind string

const (
	contextSourceBasePrompt       contextSourceKind = "base_prompt"
	contextSourceUserKnowledge    contextSourceKind = "user_knowledge"
	contextSourcePinnedMemory     contextSourceKind = "pinned_memory"
	contextSourceProjectKnowledge contextSourceKind = "project_knowledge"
	contextSourceAgentDocument    contextSourceKind = "agent_document"
	contextSourceSessionPlan      contextSourceKind = "session_plan"
	contextSourceLifecycleHook    contextSourceKind = "lifecycle_hook"
	contextSourceRecalledMemory   contextSourceKind = "recalled_memory"
	contextSourceSessionGoal      contextSourceKind = "session_goal"
)

type contextPurpose string

const (
	contextPurposeInstruction contextPurpose = "instruction"
	contextPurposeData        contextPurpose = "data"
)

func (c contextSourceKind) source(reference string) contextSource {
	return contextSource{Kind: c, Reference: reference, Purpose: c.purpose()}
}

func (c contextSourceKind) purpose() contextPurpose {
	switch c {
	case contextSourcePinnedMemory,
		contextSourceRecalledMemory,
		contextSourceSessionGoal,
		contextSourceSessionPlan:
		return contextPurposeData
	case contextSourceBasePrompt,
		contextSourceUserKnowledge,
		contextSourceProjectKnowledge,
		contextSourceAgentDocument,
		contextSourceLifecycleHook:
		return contextPurposeInstruction
	default:
		return ""
	}
}

type contextSource struct {
	Kind      contextSourceKind `json:"kind"`
	Reference string            `json:"reference,omitempty"`
	Purpose   contextPurpose    `json:"purpose"`
}

type contextProvenance struct {
	SchemaVersion uint16          `json:"schema_version"`
	Sources       []contextSource `json:"sources"`
}

func (c contextProvenance) validate() error {
	if c.SchemaVersion != contextProvenanceSchemaVersion || len(c.Sources) == 0 {
		return fmt.Errorf("agentexec: invalid context provenance schema or empty source set")
	}
	_, err := contextSources(c.Sources).provenance()
	return err
}

// replaceableSessionState reports the isolated state kind carried by this
// message. Goal and Plan must never share a message with each other or with
// frozen deployment instructions because each can change mid-Interaction.
func (c contextProvenance) replaceableSessionState() (contextSourceKind, bool, error) {
	if err := c.validate(); err != nil {
		return "", false, err
	}
	var stateKind contextSourceKind
	for _, source := range c.Sources {
		if source.Kind == contextSourceSessionGoal || source.Kind == contextSourceSessionPlan {
			stateKind = source.Kind
			break
		}
	}
	if stateKind == "" {
		return "", false, nil
	}
	if len(c.Sources) != 1 || c.Sources[0].Kind != stateKind {
		return "", false, fmt.Errorf("agentexec: replaceable Session state must be an isolated source")
	}
	return stateKind, true, nil
}

type contextSources []contextSource

func (c contextSources) provenance() (contextProvenance, error) {
	for index, source := range c {
		expectedPurpose := source.Kind.purpose()
		if expectedPurpose == "" || source.Purpose != expectedPurpose {
			return contextProvenance{}, fmt.Errorf(
				"agentexec: invalid context source[%d] kind %q purpose %q",
				index,
				source.Kind,
				source.Purpose,
			)
		}
	}
	return contextProvenance{
		SchemaVersion: contextProvenanceSchemaVersion,
		Sources:       slices.Clone(c),
	}, nil
}

func (c contextSources) attach(target *coremetadata.Map, targetName string) error {
	if len(c) == 0 {
		return nil
	}
	provenance, err := c.provenance()
	if err != nil {
		return err
	}
	if err := target.Set(contextProvenanceMetadataKey, provenance); err != nil {
		return fmt.Errorf("agentexec: attach %s context provenance: %w", targetName, err)
	}
	return nil
}

type promptSection struct {
	text    string
	sources contextSources
}

type promptComposition struct {
	sections []promptSection
}

func (p *promptComposition) append(
	text string,
	source contextSource,
	additionalSources ...contextSource,
) {
	if text == "" {
		return
	}
	sources := make(contextSources, 1, 1+len(additionalSources))
	sources[0] = source
	sources = append(sources, additionalSources...)
	p.sections = append(p.sections, promptSection{
		text: text, sources: sources,
	})
}

func (p promptComposition) render() string {
	var rendered strings.Builder
	for index, section := range p.sections {
		if index > 0 {
			rendered.WriteString("\n\n")
		}
		rendered.WriteString(section.text)
	}
	return rendered.String()
}

func (p promptComposition) sources() contextSources {
	count := 0
	for _, section := range p.sections {
		count += len(section.sources)
	}
	sources := make(contextSources, 0, count)
	for _, section := range p.sections {
		sources = append(sources, section.sources...)
	}
	return sources
}

func (p promptComposition) systemMessage() (corechat.Message, error) {
	message := corechat.NewSystemMessage(p.render())
	if err := p.sources().attach(&message.Metadata, "system message"); err != nil {
		return corechat.Message{}, err
	}
	return message, nil
}
