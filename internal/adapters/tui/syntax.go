package tui

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"

	"charm.land/lipgloss/v2"
)

// highlightCode tokenises content via chroma using a lexer chosen from the
// file path, then maps each token to a palette-colored ANSI string. Returns
// the source split into lines, each line carrying its own embedded ANSI
// styling so the viewer's per-line rendering loop can prepend line numbers
// and diff highlights without restyling the body.
//
// Returns ok=false when no lexer is available — the caller should fall back
// to plain dim text rendering. We do not stretch generic lexers across
// unrecognized extensions; surprise highlighting would mislead the eye.
func highlightCode(content, path string, p Palette) (lines []string, ok bool) {
	lexer := lexers.Match(path)
	if lexer == nil {
		// Try by content fallback for files without recognized extensions.
		// Skip when content is too small — the analyser is statistical and
		// produces false-positives for short snippets.
		if len(content) < 64 {
			return nil, false
		}
		lexer = lexers.Analyse(content) //nolint:misspell // third-party API uses British spelling
		if lexer == nil {
			return nil, false
		}
	}
	lexer = chroma.Coalesce(lexer)

	iter, err := lexer.Tokenise(nil, content)
	if err != nil {
		return nil, false
	}

	var current strings.Builder
	out := make([]string, 0, strings.Count(content, "\n")+1)
	flush := func() {
		out = append(out, current.String())
		current.Reset()
	}
	for tok := iter(); tok != chroma.EOF; tok = iter() {
		text := tok.Value
		style, styled := styleForToken(tok.Type, p)
		// Tokens may span newlines (e.g. multi-line strings, block comments).
		// Split so each output line carries its own styled segment — without
		// this, an embedded "\n" would smuggle ANSI codes between viewer
		// rows and line numbers would inherit the wrong color.
		for {
			i := strings.IndexByte(text, '\n')
			if i < 0 {
				if len(text) > 0 {
					current.WriteString(applyStyle(style, styled, text))
				}
				break
			}
			if i > 0 {
				current.WriteString(applyStyle(style, styled, text[:i]))
			}
			flush()
			text = text[i+1:]
		}
	}
	// Always flush the trailing pending line so the output line count
	// matches strings.Split(content, "\n") semantics:
	//   "a"      → ["a"]
	//   "a\n"    → ["a", ""]
	//   "a\nb"   → ["a", "b"]
	//   "a\nb\n" → ["a", "b", ""]
	// Without this final flush, files ending in "\n" lose their trailing
	// empty line and the viewer's line-number prefix drifts off-by-one.
	flush()
	return out, true
}

// applyStyle is a thin wrapper over lipgloss.Style.Render that no-ops when
// the token has no associated style — saves on ANSI codes for plain
// content. lipgloss.Style is a struct holding []color.Color slices and
// can't be compared with ==, so styleForToken signals plain via a bool.
func applyStyle(style lipgloss.Style, styled bool, text string) string {
	if !styled {
		return text
	}
	return style.Render(text)
}

// styleForToken maps a chroma token type to a lipgloss style keyed off the
// project palette. Returns (style, styled=false) for structural tokens
// (whitespace, generic text) so plain content blends in with surrounding
// viewer chrome without burning ANSI bytes.
//
// The mapping is deliberately coarse — five buckets that cover the bulk of
// what users actually scan for: keywords, strings, numbers, comments, and
// types/functions. Finer-grained roles fall through to plain. Adding more
// buckets is cheap; the cost is decision fatigue when tweaking the palette.
func styleForToken(t chroma.TokenType, p Palette) (lipgloss.Style, bool) {
	switch {
	case t.InCategory(chroma.Comment):
		return lipgloss.NewStyle().Foreground(p.TextFade).Italic(true), true
	case t.InSubCategory(chroma.LiteralString):
		return lipgloss.NewStyle().Foreground(p.Green), true
	case t.InSubCategory(chroma.LiteralNumber):
		return lipgloss.NewStyle().Foreground(p.Pink), true
	case t.InCategory(chroma.Keyword):
		return lipgloss.NewStyle().Foreground(p.Purple).Bold(true), true
	case t == chroma.NameFunction || t == chroma.NameClass || t == chroma.NameNamespace:
		return lipgloss.NewStyle().Foreground(p.Cyan), true
	case t == chroma.NameBuiltin || t == chroma.NameBuiltinPseudo:
		return lipgloss.NewStyle().Foreground(p.Cyan).Bold(true), true
	case t == chroma.NameAttribute || t == chroma.NameTag:
		return lipgloss.NewStyle().Foreground(p.Cyan), true
	case t.InCategory(chroma.Operator):
		return lipgloss.NewStyle().Foreground(p.TextMid), true
	case t == chroma.LiteralStringEscape:
		return lipgloss.NewStyle().Foreground(p.Pink).Bold(true), true
	case t == chroma.GenericHeading || t == chroma.GenericSubheading:
		// Markdown headings — chroma emits the heading body as Generic.Heading
		// which falls outside the Keyword / String / Number buckets above.
		return lipgloss.NewStyle().Foreground(p.Purple).Bold(true), true
	case t == chroma.GenericStrong:
		return lipgloss.NewStyle().Bold(true), true
	case t == chroma.GenericEmph:
		return lipgloss.NewStyle().Italic(true), true
	case t.InCategory(chroma.Generic):
		// Catch-all for the rest of markdown's Generic.* family
		// (Generic.Output, Generic.Inserted, Generic.Deleted, etc.).
		return lipgloss.NewStyle().Foreground(p.TextMid), true
	}
	return lipgloss.Style{}, false
}

// hasLexerFor reports whether highlightCode would produce output for the
// given path. Used by the viewer's renderer to decide between the
// syntax-aware path and the plain dim-text path without round-tripping
// through tokenisation.
func hasLexerFor(path string) bool {
	return lexers.Match(path) != nil
}
