package terminal

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

const fileElement headless.ElementKind = 1

const draftPersistenceDelay = 150 * time.Millisecond

type attachmentInsertion struct {
	item  agent.Attachment
	count int
}

type draftObservation struct {
	sessionID string
	message   agent.Message
	ready     bool
}

func (observation *draftObservation) Observe(sessionID string, message agent.Message) bool {
	changed := observation.ready &&
		(observation.sessionID != sessionID || !observation.message.Equal(message))
	observation.Reset(sessionID, message)
	return changed
}

func (observation *draftObservation) Reset(sessionID string, message agent.Message) {
	observation.sessionID = sessionID
	observation.message = message.Clone()
	observation.ready = true
}

// promptHistory keeps semantic messages rather than rendered editor text. A
// recalled prompt therefore restores attachment chips as attachments instead of
// turning their labels into ordinary @words.
type promptHistory struct {
	entries []agent.Message
	at      int
	draft   agent.Message
	limit   int
}

func (h *promptHistory) Load(messages []agent.Message) {
	h.entries, h.at, h.draft = nil, 0, agent.Message{}
	for _, message := range messages {
		h.Add(message)
	}
}

func (h *promptHistory) Add(message agent.Message) {
	h.at, h.draft = 0, agent.Message{}
	message = message.Clone()
	if strings.TrimSpace(message.Text) == "" && len(message.Attachments) == 0 {
		return
	}
	if len(h.entries) > 0 && h.entries[len(h.entries)-1].Equal(message) {
		return
	}
	h.entries = append(h.entries, message)
	limit := h.limit
	if limit <= 0 {
		limit = 1000
	}
	if len(h.entries) > limit {
		dropped := len(h.entries) - limit
		clear(h.entries[:dropped])
		h.entries = slices.Clone(h.entries[dropped:])
	}
}

func (h *promptHistory) Back(current agent.Message) (agent.Message, bool) {
	if h.at >= len(h.entries) {
		return agent.Message{}, false
	}
	if h.at == 0 {
		h.draft = current.Clone()
	}
	h.at++
	return h.entries[len(h.entries)-h.at].Clone(), true
}

func (h *promptHistory) Forward() (agent.Message, bool) {
	if h.at == 0 {
		return agent.Message{}, false
	}
	h.at--
	if h.at == 0 {
		draft := h.draft.Clone()
		h.draft = agent.Message{}
		return draft, true
	}
	return h.entries[len(h.entries)-h.at].Clone(), true
}

// resolveAttachment performs every fallible check before the composer is
// mutated. Completion acceptance can therefore commit the token replacement and
// attachment chip as one successful UI transition instead of destroying the
// user's token when resolution, duplicate detection, or capacity checks fail.
func (a *app) resolveAttachment(path string) (attachmentInsertion, error) {
	if a.attachments == nil {
		return attachmentInsertion{}, errors.New("attachments are unavailable in this session")
	}
	current, err := a.composerMessage()
	if err != nil {
		return attachmentInsertion{}, err
	}
	item, err := a.attachments.Resolve(a.ctx, path)
	if err != nil {
		return attachmentInsertion{}, err
	}
	if err := a.validateMessageCapabilities(agent.Message{Attachments: []agent.Attachment{item}}); err != nil {
		return attachmentInsertion{}, err
	}
	for _, attached := range current.Attachments {
		if attached.Path == item.Path {
			return attachmentInsertion{}, fmt.Errorf("%s is already attached", item.Name)
		}
	}
	if len(current.Attachments) >= agent.MaxMessageAttachments {
		return attachmentInsertion{}, fmt.Errorf("a prompt accepts at most %d attachments", agent.MaxMessageAttachments)
	}
	return attachmentInsertion{item: item, count: len(current.Attachments) + 1}, nil
}

func (a *app) insertAttachment(insertion attachmentInsertion) {
	item := insertion.item
	element := a.composer.Editor().InsertElement(fileElement, "@"+item.Name)
	a.attachmentElements[element.ID] = item
	a.message(fmt.Sprintf("attached %s · %s · %d/%d", item.Name, item.MimeType, insertion.count, agent.MaxMessageAttachments))
}

func (a *app) addAttachment(path string) error {
	insertion, err := a.resolveAttachment(path)
	if err != nil {
		return err
	}
	a.insertAttachment(insertion)
	return nil
}

func (a *app) acceptAttachmentCompletion(path string, token headless.Token) {
	insertion, err := a.resolveAttachment(path)
	if err != nil {
		a.message(err.Error())
		return
	}
	a.completionGate.Reset()
	a.composer.Editor().Replace(max(token.Start-1, 0), token.End, "")
	a.insertAttachment(insertion)
}

func (a *app) removeAttachment(argument string) error {
	argument = strings.TrimSpace(argument)
	if argument == "" {
		return errors.New("/detach needs a file name, number, or all")
	}
	elements := a.composer.Editor().Elements()
	if len(elements) == 0 {
		return errors.New("the composer has no attachments")
	}
	if strings.EqualFold(argument, "all") {
		a.removeAllAttachments(elements)
		a.message("removed all attachments")
		return nil
	}
	element, item, err := a.findAttachment(elements, argument)
	if err != nil {
		return err
	}
	a.composer.Editor().RemoveElement(element.ID)
	a.message("detached " + item.Name)
	return nil
}

func (a *app) removeAllAttachments(elements []headless.Element) {
	for _, element := range elements {
		if element.Kind != fileElement {
			continue
		}
		a.composer.Editor().RemoveElement(element.ID)
	}
}

type composerAttachment struct {
	element headless.Element
	item    agent.Attachment
}

func (a *app) findAttachment(elements []headless.Element, argument string) (headless.Element, agent.Attachment, error) {
	attached := make([]composerAttachment, 0, len(elements))
	for _, element := range elements {
		item, ok := a.attachmentElements[element.ID]
		if !ok || element.Kind != fileElement {
			continue
		}
		attached = append(attached, composerAttachment{element: element, item: item})
	}
	if position, err := strconv.Atoi(argument); err == nil {
		if position > 0 && position <= len(attached) {
			match := attached[position-1]
			return match.element, match.item, nil
		}
		return headless.Element{}, agent.Attachment{}, fmt.Errorf("attachment %q is not in the composer", argument)
	}
	if matches := matchingAttachments(attached, func(item agent.Attachment) bool { return argument == item.Name }); len(matches) > 0 {
		return uniqueAttachment(argument, matches)
	}
	if matches := matchingAttachments(attached, func(item agent.Attachment) bool { return argument == filepathBase(item.Name) }); len(matches) > 0 {
		return uniqueAttachment(argument, matches)
	}
	return headless.Element{}, agent.Attachment{}, fmt.Errorf("attachment %q is not in the composer", argument)
}

func matchingAttachments(attached []composerAttachment, matches func(agent.Attachment) bool) []composerAttachment {
	selected := make([]composerAttachment, 0, len(attached))
	for _, candidate := range attached {
		if matches(candidate.item) {
			selected = append(selected, candidate)
		}
	}
	return selected
}

func uniqueAttachment(argument string, matches []composerAttachment) (headless.Element, agent.Attachment, error) {
	if len(matches) != 1 {
		return headless.Element{}, agent.Attachment{}, fmt.Errorf("attachment %q is ambiguous; use its number or full name", argument)
	}
	return matches[0].element, matches[0].item, nil
}

func filepathBase(path string) string {
	if at := strings.LastIndexAny(path, "/\\"); at >= 0 {
		return path[at+1:]
	}
	return path
}

func (a *app) showAttachments() {
	message, err := a.composerMessage()
	if err != nil {
		a.message(err.Error())
		return
	}
	if len(message.Attachments) == 0 {
		a.message("the composer has no attachments")
		return
	}
	lines := make([]string, 0, len(message.Attachments))
	for i, item := range message.Attachments {
		lines = append(lines, fmt.Sprintf("%d. %s · %s · %d bytes", i+1, item.Name, item.MimeType, item.Size))
	}
	a.transcript.Append(&kit.Message{Theme: a.transcript.theme, Speaker: "attachments", Body: strings.Join(lines, "\n")})
}

func (a *app) composerMessage() (agent.Message, error) {
	editor := a.composer.Editor()
	lines := strings.Split(editor.Text(), "\n")
	elements := editor.Elements()
	attachments, err := a.collectAttachments(editor, elements)
	if err != nil {
		return agent.Message{}, err
	}
	if err := stripAttachmentElements(lines, elements); err != nil {
		return agent.Message{}, err
	}
	return agent.Message{Text: strings.TrimSpace(strings.Join(lines, "\n")), Attachments: attachments}, nil
}

func (a *app) collectAttachments(editor *headless.Editor, elements []headless.Element) ([]agent.Attachment, error) {
	attachments := make([]agent.Attachment, 0, len(elements))
	for _, element := range elements {
		if element.Kind != fileElement {
			continue
		}
		item, ok := a.attachmentElements[element.ID]
		if !ok {
			return nil, fmt.Errorf("attachment chip %q lost its backing value", element.Text(editor))
		}
		attachments = append(attachments, item)
	}
	return attachments, nil
}

func stripAttachmentElements(lines []string, elements []headless.Element) error {
	for _, element := range slices.Backward(elements) {
		if element.Kind != fileElement || element.Line < 0 || element.Line >= len(lines) {
			continue
		}
		line := lines[element.Line]
		if element.Start < 0 || element.End > len(line) || element.Start >= element.End {
			return errors.New("attachment chip has an invalid editor range")
		}
		end := element.End
		if end < len(line) && line[end] == ' ' {
			end++
		}
		lines[element.Line] = line[:element.Start] + line[end:]
	}
	return nil
}

func (a *app) resetComposer() {
	a.composer.Reset()
	clear(a.attachmentElements)
	a.confirmation.Reset()
}

func (a *app) rememberPrompt(message agent.Message) error {
	if a.workbench != nil {
		if err := a.workbench.Remember(message); err != nil {
			a.reportWorkbenchIssue(workbenchHistory, err)
			return err
		}
	}
	a.history.Add(message)
	a.reportWorkbenchIssue(workbenchHistory, nil)
	return nil
}

func (a *app) persistDraft() error {
	a.cancelScheduledDraftSave()
	message, _, err := a.currentDraft()
	if err == nil {
		err = a.saveDraft(message)
	}
	a.reportWorkbenchIssue(workbenchDraft, err)
	return err
}

// scheduleDraftPersistence marks authoring input in constant time. The complete
// message is captured only after the input burst settles, so editing a long
// prompt does not rebuild and clone its entire value after every key.
func (a *app) scheduleDraftPersistence() {
	if a.drafts == nil || a.session.ID == "" || a.closed {
		return
	}
	a.cancelScheduledDraftSave()
	a.stopDraftSave = a.loop.After(draftPersistenceDelay, func() {
		a.stopDraftSave = nil
		message, _, err := a.currentDraft()
		if err != nil {
			a.reportWorkbenchIssue(workbenchDraft, err)
			return
		}
		if a.draftState.Observe(a.session.ID, message) {
			a.drafts.Schedule(a.session.ID, message)
		}
	})
}

func (a *app) cancelScheduledDraftSave() {
	if a.stopDraftSave == nil {
		return
	}
	a.stopDraftSave()
	a.stopDraftSave = nil
}

func (a *app) saveDraft(message agent.Message) error {
	if a.drafts == nil || a.session.ID == "" {
		return nil
	}
	a.cancelScheduledDraftSave()
	a.draftState.Reset(a.session.ID, message)
	return a.drafts.Flush(a.session.ID, message)
}

func (a *app) restoreComposer(message agent.Message) {
	a.resetComposer()
	for _, item := range message.Attachments {
		element := a.composer.Editor().InsertElement(fileElement, "@"+item.Name)
		a.attachmentElements[element.ID] = item
	}
	a.composer.Editor().Insert(message.Text)
}

func (a *app) recallPrevious() bool {
	current, err := a.composerMessage()
	if err != nil {
		a.message(err.Error())
		return true
	}
	message, ok := a.history.Back(current)
	if ok {
		a.restoreComposer(message)
	}
	return ok
}

func (a *app) recallNext() bool {
	message, ok := a.history.Forward()
	if ok {
		a.restoreComposer(message)
	}
	return ok
}
