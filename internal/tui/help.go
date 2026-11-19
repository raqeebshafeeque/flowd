package tui

import "strings"

// Help is a page of every key, plus the full wording of the last error, which
// stays here after its popup has gone. j and k scroll it.

var helpSections = []struct {
	title string
	keys  [][2]string
}{
	{"Moving around", [][2]string{
		{"j / k", "Select the next or previous item"},
		{"enter / v", "Show or hide the selected item's details"},
		{"c", "Open the calendar to look at another day"},
		{"t", "Come back to today"},
		{"r", "Review the day on screen, its week or its month"},
		{"? ", "Open or close this help"},
		{"q", "Quit (Ctrl+C always quits)"},
	}},
	{"Recording work (today only)", [][2]string{
		{"space", "Start, pause or resume the selected item"},
		{"f", "Finish it for today"},
		{"s", "Skip it for today"},
		{"u", "Bring back a skipped item"},
		{"R", "Reopen a finished item"},
	}},
	{"Planning (today and later)", [][2]string{
		{"n", "Add an item"},
		{"e", "Edit the selected item"},
		{"d", "Delete the selected item, after asking"},
	}},
	{"In a form", [][2]string{
		{"tab / enter", "Move to the other field. Enter never saves"},
		{"ctrl+s", "Save"},
		{"esc", "Go back, asking first if something was typed"},
	}},
	{"In the calendar", [][2]string{
		{"h / l", "Previous or next day"},
		{"j / k", "Next or previous week"},
		{"m", "Type a month number, 1 to 12"},
		{"y", "Type a year"},
		{"t", "Today"},
		{"enter", "Show the chosen day"},
	}},
	{"In the review", [][2]string{
		{"1 / 2 / 3", "The day, its week or its month. Tab goes to the next one"},
		{"h / l", "The previous or next day, week or month"},
		{"t", "Come back to now"},
		{"j / k", "Scroll"},
		{"esc / r", "Go back to the list"},
	}},
}

func (m *model) toggleHelp() {
	m.help, m.helpOffset = !m.help, 0
}

func (m *model) helpKey(key string) {
	switch key {
	case "j", "down":
		m.helpOffset++
	case "k", "up":
		m.helpOffset = max(0, m.helpOffset-1)
	case "esc", "?":
		m.help = false
	}
}

// helpRows draws the whole page, before scrolling.
func (m *model) helpRows(width int) []string {
	var rows []string
	// One empty row above the first section and two between sections.
	section := func(title string) {
		if len(rows) > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, "", " "+m.style.paint("heading", title), " "+m.style.paint("border", strings.Repeat("─", width-2)))
	}
	if m.lastError != nil {
		f := m.lastError
		section("Last error")
		rows = append(rows, " "+m.style.paint("error", f.Summary))
		for _, line := range wrap(f.Hint, width-2) {
			if line != "" {
				rows = append(rows, " "+line)
			}
		}
	}
	for _, s := range helpSections {
		section(s.title)
		for _, k := range s.keys {
			rows = append(rows, " "+fit(k[0], 13)+k[1])
		}
	}
	return rows
}

func (m *model) helpBody(width, height int) []string {
	rows := m.helpRows(width)
	m.helpOffset = min(max(m.helpOffset, 0), max(0, len(rows)-height))
	return pad(rows[m.helpOffset:], height)
}
