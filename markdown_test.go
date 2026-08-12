package main

import (
	"strings"
	"testing"

	markterm "github.com/memmaker/gollum/mark-term"
)

// The narration file is read as it stands, so every mark the DM writes has to
// come off while the words it wrapped stay put.
func TestPlainTextStripsMarkdown(t *testing.T) {
	src := "## The Hall\n\n" +
		"**Attributes:** Strength *Mediocre*, Agility `Fair`.  \n" +
		"A sign reads: **REVERSE**.\n\n" +
		"- red statue\n- blue statue\n\n" +
		"> Dust lies undisturbed.\n\n" +
		"See [the door](http://example.com/door)."
	want := "The Hall\n\n" +
		"Attributes: Strength Mediocre, Agility Fair. A sign reads: REVERSE.\n\n" +
		"red statue\n\nblue statue\n\n" +
		"Dust lies undisturbed.\n\n" +
		"See the door."
	if got := plainText(src); got != want {
		t.Fatalf("plainText:\n got %q\nwant %q", got, want)
	}
}

// A stray inline tag used to panic the terminal renderer: RawHTML is an inline
// node, and gollum asked it for Lines(). One line of narration is enough to
// catch it coming back.
func TestInlineHTMLRenders(t *testing.T) {
	const prose = `The door opens. <img src="x.png" alt="an open door">`
	if got := markterm.RenderMarkdown(prose, 80); !strings.Contains(got, "The door opens.") {
		t.Fatalf("inline tag lost the prose: %q", got)
	}
	if got := plainText(prose); got != "The door opens." {
		t.Fatalf("plainText kept the tag: %q", got)
	}
}
