package terminal

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

const fileElement headless.ElementKind = 1

// promptHistory keeps semantic messages rather than rendered editor text. A
// recalled prompt therefore restores attachment chips as attachments instead of
// turning their labels into ordinary @words.
type promptHistory struct {
	entries []agent.Message
	at      int
	draft   agent.Message
	limit   int
}

func (h *promptHistory) Add(message agent.Message) {
	h.at, h.draft = 0, agent.Message{}
	message = message.Clone()
	if strings.TrimSpace(message.Text) == "" && len(message.Attachments) == 0 {
		return
	}
	if len(h.entries) > 0 && equalMessage(h.entries[len(h.entries)-1], message) {
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

func equalMessage(a, b agent.Message) bool {
	return a.Text == b.Text && slices.EqualFunc(a.Attachments, b.Attachments, func(a, b agent.Attachment) bool { return a.ID == b.ID })
}

func (a *app) addAttachment(path string) error {
	if a.attachments == nil {
		return errors.New("attachments are unavailable in this session")
	}
	current, err := a.composerMessage()
	if err != nil {
		return err
	}
	item, err := a.attachments.Resolve(a.ctx, path)
	if err != nil {
		return err
	}
	for _, attached := range current.Attachments {
		if attached.Path == item.Path {
			return fmt.Errorf("%s is already attached", item.Name)
		}
	}
	if len(current.Attachments) >= agent.MaxMessageAttachments {
		return fmt.Errorf("a prompt accepts at most %d attachments", agent.MaxMessageAttachments)
	}
	element := a.composer.Editor().InsertElement(fileElement, "@"+item.Name)
	if element.ID == 0 {
		return errors.New("could not insert attachment into the composer")
	}
	a.attachmentElements[element.ID] = item
	a.message(fmt.Sprintf("attached %s · %s · %d/%d", item.Name, item.MimeType, len(current.Attachments)+1, agent.MaxMessageAttachments))
	return nil
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
	element, item, found := a.findAttachment(elements, argument)
	if !found {
		return fmt.Errorf("attachment %q is not in the composer", argument)
	}
	a.composer.Editor().RemoveElement(element.ID)
	delete(a.attachmentElements, element.ID)
	a.message("detached " + item.Name)
	return nil
}

func (a *app) removeAllAttachments(elements []headless.Element) {
	for _, element := range elements {
		if element.Kind != fileElement {
			continue
		}
		a.composer.Editor().RemoveElement(element.ID)
		delete(a.attachmentElements, element.ID)
	}
}

func (a *app) findAttachment(elements []headless.Element, argument string) (headless.Element, agent.Attachment, bool) {
	position := 0
	for _, element := range elements {
		item, ok := a.attachmentElements[element.ID]
		if !ok || element.Kind != fileElement {
			continue
		}
		position++
		if attachmentMatches(argument, position, item) {
			return element, item, true
		}
	}
	return headless.Element{}, agent.Attachment{}, false
}

func attachmentMatches(argument string, position int, item agent.Attachment) bool {
	return argument == strconv.Itoa(position) || argument == item.Name || argument == filepathBase(item.Name)
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
