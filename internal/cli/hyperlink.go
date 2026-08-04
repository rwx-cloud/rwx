package cli

import (
	"os"

	"github.com/savioxavier/termlink"
)

const hyperlinkColor = "cyan underline"

// HyperlinksSupported reports whether the terminal can render an OSC 8 hyperlink.
// Terminals without OSC 8 support swallow the sequence, which hides the URL.
func HyperlinksSupported(tty bool) bool {
	return tty && termlink.SupportsHyperlinks()
}

// Hyperlink renders label as an OSC 8 hyperlink to url, or returns url on its own
// when the terminal cannot render one.
func Hyperlink(label, url string, tty bool) string {
	if !HyperlinksSupported(tty) {
		return url
	}

	if os.Getenv("NO_COLOR") != "" {
		return termlink.Link(label, url)
	}

	return termlink.ColorLink(label, url, hyperlinkColor)
}
