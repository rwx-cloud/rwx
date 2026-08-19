package cli

import (
	"bytes"
	"strings"
	"testing"

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

	t.Run("rejects an empty multi-selection", func(t *testing.T) {
		picker := newBubbleTeaChoicePicker(bytes.NewBufferString("\r"), new(strings.Builder))

		selected, err := picker.PickMany("Select tool caches:", choices)

		require.Nil(t, selected)
		require.EqualError(t, err, "no options selected")
	})
}
