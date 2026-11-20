package tui

import (
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

// labelWidth is the room for "Duration" and its gap before an input.
const labelWidth = 11

// field is one line of literal text with a caret.
type field struct {
	text  []rune
	caret int
}

func newField(text string) field {
	runes := []rune(text)
	return field{text: runes, caret: len(runes)}
}

func (f *field) value() string { return string(f.text) }

func (f *field) insert(text string) {
	text = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		return r
	}, text)
	runes := []rune(text)
	f.text = append(f.text[:f.caret], append(runes, f.text[f.caret:]...)...)
	f.caret += len(runes)
}

// edit handles keys that move the caret or delete. It reports whether it
// used the key.
func (f *field) edit(key string) bool {
	switch key {
	case "left":
		f.caret = max(0, f.caret-1)
	case "right":
		f.caret = min(len(f.text), f.caret+1)
	case "home", "ctrl+a":
		f.caret = 0
	case "end", "ctrl+e":
		f.caret = len(f.text)
	case "backspace":
		if f.caret > 0 {
			f.text = append(f.text[:f.caret-1], f.text[f.caret:]...)
			f.caret--
		}
	case "delete":
		if f.caret < len(f.text) {
			f.text = append(f.text[:f.caret], f.text[f.caret+1:]...)
		}
	default:
		return false
	}
	return true
}

// window returns the part of the text that fits in width cells, keeping the
// caret visible, and the caret's column inside it.
func (f *field) window(width int) (string, int) {
	start := 0
	for ansi.StringWidth(string(f.text[start:f.caret])) >= width {
		start++
	}
	visible := ansi.Truncate(string(f.text[start:]), width, "")
	return visible, ansi.StringWidth(string(f.text[start:f.caret]))
}

// form is the New or Edit panel. Typing always goes into the focused field,
// so letters never trigger actions while a form is open.
type form struct {
	edit     bool
	itemID   domain.ID
	revision planner.Revision
	fields   [2]field // title, duration
	original [2]string
	focus    int
	errors   [2]string
	saving   bool
	discard  bool // asking whether to throw away changes
}

func (f *form) dirty() bool {
	return f.fields[0].value() != f.original[0] || f.fields[1].value() != f.original[1]
}

// openNew starts a blank form for a new item.
func (m *model) openNew() {
	if m.busy() || !m.loaded {
		return
	}
	m.form, m.details = &form{}, false
}

// openEdit starts a form holding the selected item's title and Duration.
func (m *model) openEdit() {
	i := m.index()
	if i < 0 || m.busy() {
		return
	}
	item := m.state.View.Items[i]
	duration, _ := domain.FormatDuration(item.DurationMinutes)
	f := &form{edit: true, itemID: item.ID, revision: m.state.Revisions[item.ID], original: [2]string{item.Title, duration}}
	f.fields = [2]field{newField(item.Title), newField(duration)}
	m.form, m.details = f, false
}

// formKey handles every key while a form is open.
func (m *model) formKey(msg tea.KeyPressMsg) tea.Cmd {
	f := m.form
	if f.saving || m.busy() {
		return nil
	}
	key := msg.String()
	if f.discard {
		switch key {
		case "y":
			m.form = nil
		case "n", "esc":
			f.discard = false
		}
		return nil
	}
	switch key {
	case "ctrl+s":
		return m.save()
	case "esc":
		if f.dirty() {
			f.discard = true
		} else {
			m.form = nil
		}
	case "tab", "shift+tab", "up", "down":
		f.focus = 1 - f.focus
	case "enter":
		f.focus = 1 // Enter moves on, it never saves
	default:
		if !f.fields[f.focus].edit(key) && msg.Text != "" {
			f.fields[f.focus].insert(msg.Text)
		}
	}
	return nil
}

// paste inserts pasted text into the focused field as one line.
func (m *model) paste(text string) {
	if m.form != nil && !m.form.saving && !m.form.discard {
		m.form.fields[m.form.focus].insert(text)
	}
}

// save sends the form. An unchanged edit just closes it.
func (m *model) save() tea.Cmd {
	f := m.form
	if f.edit && !f.dirty() {
		m.form = nil
		return nil
	}
	title, duration := f.fields[0].value(), f.fields[1].value()
	intent := planner.Intent{Operation: planner.Add, Selection: planner.Selection{Date: m.date}, Title: &title, Duration: &duration}
	if f.edit {
		intent = planner.Intent{Operation: planner.Edit, Selection: planner.Selection{Date: m.date}, ItemID: f.itemID, Revision: f.revision}
		if title != f.original[0] {
			intent.Title = &title
		}
		if duration != f.original[1] {
			intent.Duration = &duration
		}
	}
	f.saving, f.errors, m.pending = true, [2]string{}, true
	return func() tea.Msg {
		r, err := m.client.Do(m.ctx, intent)
		return actionMsg{r, err}
	}
}

// formFinished handles the result of a save. A problem with one field is
// shown under that field, keeping everything typed. Anything else pops up.
func (m *model) formFinished(msg actionMsg) {
	f := m.form
	f.saving = false
	switch {
	case msg.result.Outcome == planner.Confirmed:
		m.selected, m.form = msg.result.Item.ID, nil
	case errors.Is(msg.err, domain.ErrInvalidTitle):
		f.errors[0], f.focus = describe(msg.err), 0
	case errors.Is(msg.err, domain.ErrInvalidDuration):
		f.errors[1], f.focus = describe(msg.err), 1
	}
}

func describe(err error) string {
	fb := planner.Describe(err)
	if fb.Hint == "" {
		return fb.Summary
	}
	return fb.Summary + ". " + fb.Hint
}

// formRows draws the form panel and returns where the caret is, as a row
// and column inside the panel.
func (m *model) formRows(width, height int) ([]string, int, int) {
	f := m.form
	heading := "Add item"
	if f.edit {
		heading = "Edit item"
	}
	rows := []string{" " + m.style.paint("heading", heading), " " + m.style.paint("border", strings.Repeat("─", width-2))}
	inputWidth := width - labelWidth - 2
	caretRow, caretCol := 0, 0
	for i, name := range []string{"Title", "Duration"} {
		if i > 0 {
			rows = append(rows, "") // a little room between the fields
		}
		text, col := f.fields[i].window(inputWidth)
		if i == 1 && text == "" && f.focus != 1 {
			text = m.style.paint("muted", "e.g. 30m, 1h, 1h30m")
		}
		if i == f.focus {
			caretRow, caretCol = len(rows), 1+labelWidth+col
		}
		label := name
		if i == f.focus {
			label = m.style.paint("selected", name)
		}
		rows = append(rows, " "+label+strings.Repeat(" ", labelWidth-len(name))+text)
		for _, line := range wrap(f.errors[i], inputWidth) {
			if line != "" {
				rows = append(rows, strings.Repeat(" ", 1+labelWidth)+m.style.paint("error", line))
			}
		}
	}
	return pad(rows, height), caretRow, caretCol
}

// formFooter replaces the usual footer while a form is open.
func (m *model) formFooter() string {
	switch {
	case m.form.discard:
		return " Discard changes?  yes [y]  no [n]"
	case m.form.saving:
		return " Saving…"
	}
	return " save [ctrl+s]  next field [tab]  back [esc]"
}
