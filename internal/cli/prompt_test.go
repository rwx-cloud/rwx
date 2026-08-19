package cli

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestBubbleTeaChoicePicker(t *testing.T) {
	choices := []Choice{
		{Label: "bundler", Description: "Used by test"},
		{Label: "golang", Description: "Used by build and test"},
	}

	t.Run("selects multiple choices with space and arrow keys", func(t *testing.T) {
		input := bytes.NewBufferString(" \x1b[B \r")
		output := new(strings.Builder)
		picker := newBubbleTeaChoicePicker(input, output)

		selected, err := picker.PickMany("Select tool caches:", choices)

		require.NoError(t, err)
		require.Equal(t, []int{0, 1}, selected)
		require.Contains(t, output.String(), "space toggle")
		require.Contains(t, output.String(), "enter confirm")
	})

	t.Run("keeps the prompt open after an empty multi-selection", func(t *testing.T) {
		model := newSelectionPromptModel("Select tool caches:", choices, true)

		updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		result := updated.(selectionPromptModel)

		require.Nil(t, command)
		require.False(t, result.submitted)
		require.Contains(t, result.View(), "Select at least one option.")
	})
}
