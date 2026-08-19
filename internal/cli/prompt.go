package cli

import (
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type Choice struct {
	Label       string
	Description string
}

type ChoicePicker interface {
	PickOne(title string, choices []Choice) (int, error)
	PickMany(title string, choices []Choice) ([]int, error)
}

type bubbleTeaChoicePicker struct {
	input  io.Reader
	output io.Writer
}

type selectionPromptModel struct {
	title     string
	choices   []Choice
	cursor    int
	selected  map[int]bool
	multiple  bool
	submitted bool
	cancelled bool
}

func newBubbleTeaChoicePicker(input io.Reader, output io.Writer) ChoicePicker {
	return bubbleTeaChoicePicker{input: input, output: output}
}

func newSelectionPromptModel(title string, choices []Choice, multiple bool) selectionPromptModel {
	return selectionPromptModel{
		title:    title,
		choices:  choices,
		selected: make(map[int]bool),
		multiple: multiple,
	}
}

func (m selectionPromptModel) Init() tea.Cmd {
	return nil
}

func (m selectionPromptModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.choices)-1 {
			m.cursor++
		}
	case " ":
		if m.multiple {
			m.selected[m.cursor] = !m.selected[m.cursor]
		}
	case "enter":
		if !m.multiple {
			m.selected[m.cursor] = true
		}
		m.submitted = true
		return m, tea.Quit
	case "esc", "q", "ctrl+c":
		m.cancelled = true
		return m, tea.Quit
	}

	return m, nil
}

func (m selectionPromptModel) View() string {
	var view strings.Builder
	view.WriteString(m.title)
	view.WriteString("\n")

	for index, choice := range m.choices {
		cursor := "  "
		if index == m.cursor {
			cursor = "> "
		}

		if m.multiple {
			mark := "[ ]"
			if m.selected[index] {
				mark = "[x]"
			}
			fmt.Fprintf(&view, "%s%s %s\n", cursor, mark, choice.Label)
		} else {
			fmt.Fprintf(&view, "%s%s\n", cursor, choice.Label)
		}

		if choice.Description != "" {
			fmt.Fprintf(&view, "     %s\n", choice.Description)
		}
	}

	view.WriteString("\n↑/↓ move • ")
	if m.multiple {
		view.WriteString("space toggle • enter confirm")
	} else {
		view.WriteString("enter select")
	}
	view.WriteString(" • esc cancel\n")
	return view.String()
}

func (p bubbleTeaChoicePicker) PickOne(title string, choices []Choice) (int, error) {
	selected, err := p.run(title, choices, false)
	if err != nil {
		return 0, err
	}
	return selected[0], nil
}

func (p bubbleTeaChoicePicker) PickMany(title string, choices []Choice) ([]int, error) {
	return p.run(title, choices, true)
}

func (p bubbleTeaChoicePicker) run(title string, choices []Choice, multiple bool) ([]int, error) {
	if len(choices) == 0 {
		return nil, errors.New("no options are available")
	}

	program := tea.NewProgram(
		newSelectionPromptModel(title, choices, multiple),
		tea.WithInput(p.input),
		tea.WithOutput(p.output),
		tea.WithoutSignals(),
	)
	finalModel, err := program.Run()
	if err != nil {
		return nil, errors.Wrap(err, "unable to run interactive prompt")
	}

	result := finalModel.(selectionPromptModel)
	if result.cancelled || !result.submitted {
		return nil, errors.New("selection cancelled")
	}

	selected := make([]int, 0, len(result.selected))
	for index := range choices {
		if result.selected[index] {
			selected = append(selected, index)
		}
	}
	if len(selected) == 0 {
		return nil, errors.New("no options selected")
	}
	return selected, nil
}
