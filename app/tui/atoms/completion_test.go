package atoms

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/tui/primitives/grid"
	"github.com/Tangerg/lynx/app/tui/primitives/input"
)

var (
	file    = Trigger{Prefix: "@"}
	command = Trigger{Prefix: "/", AtStart: true}
)

func TestTokenAt(t *testing.T) {
	for _, tc := range []struct {
		what     string
		line     string
		cursor   int
		triggers []Trigger
		want     string // "start:end:query" or "" for no token
	}{
		{"a mention being typed", "look at @src/ma", 15, []Trigger{file}, "9:15:src/ma"},
		{"the prefix alone", "@", 1, []Trigger{file}, "1:1:"},
		{"a command at the start", "/hel", 4, []Trigger{command}, "1:4:hel"},
		{"a command not at the start", "and /hel", 8, []Trigger{command}, ""},
		{"a slash inside a path", "@src/ma", 7, []Trigger{file, command}, "1:7:src/ma"},
		{
			"the rightmost trigger wins", "/open @src", 10,
			[]Trigger{file, command}, "7:10:src",
		},
		{"an email address", "write to me@example.com", 23, []Trigger{file}, ""},
		{"the cursor before the trigger", "@src", 0, []Trigger{file}, ""},
		{
			"the cursor inside the token replaces all of it", "@source", 4,
			[]Trigger{file}, "1:7:sou",
		},
		{"the cursor past the token", "@src and more", 9, []Trigger{file}, ""},
		{"a token that ends at a space", "@src more", 4, []Trigger{file}, "1:4:src"},
		{"nothing to complete", "just words", 10, []Trigger{file, command}, ""},
		{"no triggers at all", "@src", 4, nil, ""},
		{"an empty trigger", "@src", 4, []Trigger{{}}, ""},
	} {
		token, ok := TokenAt(tc.line, tc.cursor, tc.triggers...)
		got := ""
		if ok {
			got = strings.Join([]string{
				strconv.Itoa(token.Start), strconv.Itoa(token.End), token.Query,
			}, ":")
		}
		if got != tc.want {
			t.Errorf("%s: TokenAt(%q, %d) = %q, want %q", tc.what, tc.line, tc.cursor, got, tc.want)
		}
	}
}

func TestTokenAtIsClampedToTheLine(t *testing.T) {
	// A cursor from somewhere that has since changed the text must not index out of it.
	for _, cursor := range []int{-5, 100} {
		if _, ok := TokenAt("@src", cursor, file); ok && cursor < 0 {
			t.Errorf("cursor %d found a token", cursor)
		}
	}
}

// offer opens a completion on a token with plain candidates.
func offer(c *Completion, texts ...string) Token {
	token := Token{Start: 1, End: 4, Query: "src", Trigger: file}
	candidates := make([]Candidate, len(texts))
	for i, s := range texts {
		candidates[i] = Candidate{Text: s}
	}
	c.Offer(token, candidates)
	return token
}

func TestAnEmptyOfferIsADismissal(t *testing.T) {
	// A popup with nothing in it is a popup in the way.
	var c Completion
	offer(&c, "one")
	c.Offer(Token{}, nil)
	if c.Open() {
		t.Fatal("an offer of nothing left the completion open")
	}
	if c.Height(20) != 0 {
		t.Fatal("a closed completion asked for room")
	}
}

func TestAClosedCompletionAnswersNothing(t *testing.T) {
	// It would be a completion with opinions about text it is not offering anything for.
	var c Completion
	c.Accept = func(Candidate, Token) { t.Fatal("a closed completion accepted") }
	for _, ev := range []input.Event{
		input.Key{Code: input.Down},
		input.Key{Code: input.Tab},
		input.Key{Code: input.Esc},
		input.Key{Code: input.Enter},
	} {
		if c.Handle(ev) {
			t.Errorf("a closed completion consumed %v", ev)
		}
	}
}

func TestAcceptingReportsTheCandidateAndTheTokenItReplaces(t *testing.T) {
	var c Completion
	var got Candidate
	var at Token
	c.Accept = func(candidate Candidate, token Token) { got, at = candidate, token }
	token := offer(&c, "src/one.go", "src/two.go")

	c.Handle(input.Key{Code: input.Down})
	if !c.Handle(input.Key{Code: input.Tab}) {
		t.Fatal("accepting was not handled")
	}
	if got.Text != "src/two.go" {
		t.Fatalf("accepted %q, want the one under the cursor", got.Text)
	}
	if at != token {
		t.Fatalf("token = %+v, want %+v", at, token)
	}
	if c.Open() {
		t.Fatal("the completion stayed open after accepting")
	}
}

func TestAcceptingClosesBeforeTheCallbackRuns(t *testing.T) {
	// The callback is about to change the text the token came from, and that change
	// must not read as another query for a completion that is still open.
	var c Completion
	openDuring := true
	c.Accept = func(Candidate, Token) { openDuring = c.Open() }
	offer(&c, "one")
	c.Handle(input.Key{Code: input.Tab})
	if openDuring {
		t.Fatal("the completion was still open while the text was being changed")
	}
}

func TestWithNowhereToSendItAcceptingIsNotConsumed(t *testing.T) {
	// A key swallowed by a widget that then does nothing with it is a key the user
	// pressed for no reason.
	var c Completion
	offer(&c, "one")
	if c.Handle(input.Key{Code: input.Tab}) {
		t.Fatal("accepting was consumed with nothing to accept into")
	}
	if !c.Open() {
		t.Fatal("the completion closed anyway")
	}
}

func TestDismissing(t *testing.T) {
	var c Completion
	offer(&c, "one", "two")
	if !c.Handle(input.Key{Code: input.Esc}) {
		t.Fatal("escape was not handled")
	}
	if c.Open() {
		t.Fatal("escape left the completion open")
	}
}

func TestMovingThroughTheCandidates(t *testing.T) {
	var c Completion
	offer(&c, "one", "two", "three")
	if got, _ := c.Current(); got.Text != "one" {
		t.Fatalf("opens on %q, want the first", got.Text)
	}
	c.Handle(input.Key{Code: input.Down})
	c.Handle(input.Key{Code: input.Down})
	if got, _ := c.Current(); got.Text != "three" {
		t.Fatalf("after two moves down = %q", got.Text)
	}
	// Not past the end: the list does not wrap, because in a long one wrapping loses
	// the reader's place.
	c.Handle(input.Key{Code: input.Down})
	if got, _ := c.Current(); got.Text != "three" {
		t.Fatalf("moved past the last candidate to %q", got.Text)
	}
}

func TestANewOfferStartsAtTheFirstCandidate(t *testing.T) {
	// The query changed, so which candidate was under the cursor answers a question
	// nobody asked.
	var c Completion
	offer(&c, "one", "two")
	c.Handle(input.Key{Code: input.Down})
	offer(&c, "alpha", "beta")
	if got, _ := c.Current(); got.Text != "alpha" {
		t.Fatalf("a new offer opens on %q, want the first", got.Text)
	}
}

func TestTheHeightIsARowPerCandidateUpToTheCap(t *testing.T) {
	var c Completion
	c.MaxRows = 3
	offer(&c, "one", "two")
	if got := c.Height(20); got != 2 {
		t.Fatalf("height = %d, want a row each", got)
	}
	offer(&c, "one", "two", "three", "four", "five")
	if got := c.Height(20); got != 3 {
		t.Fatalf("height = %d, want the cap", got)
	}
}

func TestTheWidthFitsTheWidestRow(t *testing.T) {
	var c Completion
	c.Offer(Token{}, []Candidate{
		{Text: "short"},
		{Text: "much longer one", Detail: "dir"},
	})
	if got, want := c.Width(), len("much longer one")+detailGap+len("dir"); got != want {
		t.Fatalf("width = %d, want %d", got, want)
	}
}

func TestARowShowsItsLabelAndDetail(t *testing.T) {
	var c Completion
	c.Offer(Token{}, []Candidate{{Text: "src/main.go", Label: "main.go", Detail: "src"}})
	rows := paint(20, 1, c.Draw)
	if !strings.Contains(rows[0], "main.go") {
		t.Fatalf("row = %q, want the label rather than the text", rows[0])
	}
	if !strings.HasSuffix(strings.TrimRight(rows[0], "."), "src") {
		t.Fatalf("row = %q, want the detail on the right", rows[0])
	}
}

func TestARowWithNoLabelShowsWhatItWouldInsert(t *testing.T) {
	var c Completion
	c.Offer(Token{}, []Candidate{{Text: "insert-me"}})
	if rows := paint(20, 1, c.Draw); !strings.Contains(rows[0], "insert-me") {
		t.Fatalf("row = %q", rows[0])
	}
}

func TestTheMatchedCharactersArePickedOut(t *testing.T) {
	var c Completion
	c.MatchStyle = grid.Style{Attr: grid.Bold}
	c.Offer(Token{}, []Candidate{{Text: "status", Matched: []int{0, 1}}})

	s := grid.NewSurface(20, 1)
	c.Draw(s.View())
	for x, want := range []bool{true, true, false, false, false, false} {
		if got := s.CellAt(x, 0).Style.Attr.Has(grid.Bold); got != want {
			t.Errorf("column %d emphasised = %v, want %v", x, got, want)
		}
	}
}

func TestAMatchInsideAClusterEmphasisesTheWholeCluster(t *testing.T) {
	// A pattern character can match a combining mark, whose offset is inside the
	// cluster rather than at its start. Testing for equality there would leave the
	// character the reader sees unhighlighted.
	var c Completion
	c.MatchStyle = grid.Style{Attr: grid.Bold}
	// "e" plus a combining acute: one cluster, two runes, the mark at offset 1.
	c.Offer(Token{}, []Candidate{{Text: "éx", Matched: []int{1}}})

	s := grid.NewSurface(10, 1)
	c.Draw(s.View())
	if !s.CellAt(0, 0).Style.Attr.Has(grid.Bold) {
		t.Fatal("the matched cluster was not emphasised")
	}
	if s.CellAt(1, 0).Style.Attr.Has(grid.Bold) {
		t.Fatal("the cluster after the match was emphasised")
	}
}

func TestTheSelectedRowIsTheOneUnderTheCursor(t *testing.T) {
	var c Completion
	c.SelectedStyle = grid.Style{Attr: grid.Reverse}
	offer(&c, "one", "two")
	c.Handle(input.Key{Code: input.Down})

	s := grid.NewSurface(20, 2)
	c.Draw(s.View())
	if s.CellAt(0, 0).Style.Attr.Has(grid.Reverse) {
		t.Error("the row that is not selected is drawn as selected")
	}
	if !s.CellAt(0, 1).Style.Attr.Has(grid.Reverse) {
		t.Error("the selected row is not drawn as selected")
	}
}

func TestADetailWithNoRoomIsDropped(t *testing.T) {
	// Half a description reads as a broken label. None of this may overflow the row.
	var c Completion
	c.Offer(Token{}, []Candidate{{Text: "a-fairly-long-label", Detail: "and a description"}})
	rows := paint(12, 1, c.Draw)
	if len(rows[0]) != 12 {
		t.Fatalf("row = %q, want twelve columns", rows[0])
	}
}

func TestACompletionWithNoRoomDrawsNothing(t *testing.T) {
	var c Completion
	offer(&c, "one", "two")
	for _, size := range [][2]int{{0, 0}, {1, 1}, {4, 1}} {
		paint(size[0], size[1], c.Draw)
	}
}

func TestAcceptingACompletionIsOneUndoStep(t *testing.T) {
	// Which is the reason Editor.Replace exists: taking back one thing the user did
	// should not take two.
	e := NewEditor()
	e.SetText("look at @src")
	token, ok := TokenAt(e.Text(), len("look at @src"), file)
	if !ok {
		t.Fatal("no token")
	}
	e.Replace(token.Start, token.End, "src/main.go")
	if got := e.Text(); got != "look at @src/main.go" {
		t.Fatalf("after accepting = %q", got)
	}
	if _, col := e.Cursor(); col != len("look at @src/main.go") {
		t.Fatalf("cursor at %d, want after what was put in", col)
	}
	e.Undo()
	if got := e.Text(); got != "look at @src" {
		t.Fatalf("after one undo = %q, want what was there before", got)
	}
}

func TestReplaceIsClampedToTheLine(t *testing.T) {
	e := NewEditor()
	e.SetText("abc")
	e.Replace(-3, 99, "x")
	if got := e.Text(); got != "x" {
		t.Fatalf("text = %q", got)
	}
}
