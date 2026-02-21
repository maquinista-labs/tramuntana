package render

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// MarkdownV2 special characters that must be escaped outside formatted regions.
const mdv2Special = `_*[]()~` + "`" + `>#+-=|{}.!\`

var reExpQuote = regexp.MustCompile(regexp.QuoteMeta(ExpQuoteStart) + `([\s\S]*?)` + regexp.QuoteMeta(ExpQuoteEnd))

// segment represents a piece of text that is either an expandable quote or regular content.
type segment struct {
	isQuote bool
	content string
}

// ToMarkdownV2 converts standard Markdown to Telegram MarkdownV2 format.
// Expandable quotes are extracted first (they use a custom format), then the
// rest is parsed via goldmark and rendered with a custom MarkdownV2 renderer.
func ToMarkdownV2(text string) string {
	segments := extractExpandableQuotes(text)

	var b strings.Builder
	for _, seg := range segments {
		if seg.isQuote {
			// Blockquotes must start on a new line
			if b.Len() > 0 {
				s := b.String()
				if s[len(s)-1] != '\n' {
					b.WriteString("\n")
				}
			}
			b.WriteString(renderExpandableQuote(seg.content))
		} else {
			b.WriteString(convertWithGoldmark(seg.content, false))
		}
	}

	return b.String()
}

// ToPlainText strips all markdown formatting and returns raw text.
func ToPlainText(text string) string {
	// Remove expandable quote markers
	result := strings.ReplaceAll(text, ExpQuoteStart, "")
	result = strings.ReplaceAll(result, ExpQuoteEnd, "")

	return convertWithGoldmark(result, true)
}

// SplitMessage splits text on newline boundaries to fit within maxLen.
// Messages containing expandable quotes are not split (kept atomic).
func SplitMessage(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}

	// Don't split messages with expandable quotes
	if strings.Contains(text, ExpQuoteStart) {
		return []string{text}
	}

	var parts []string
	lines := strings.Split(text, "\n")
	var current strings.Builder

	for _, line := range lines {
		// If adding this line would exceed maxLen, flush current
		if current.Len() > 0 && current.Len()+1+len(line) > maxLen {
			parts = append(parts, current.String())
			current.Reset()
		}

		// If a single line exceeds maxLen, split it by force
		if len(line) > maxLen {
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
			// Force-split the long line
			for len(line) > maxLen {
				parts = append(parts, line[:maxLen])
				line = line[maxLen:]
			}
			if len(line) > 0 {
				current.WriteString(line)
			}
			continue
		}

		if current.Len() > 0 {
			current.WriteString("\n")
		}
		current.WriteString(line)
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// extractExpandableQuotes splits text into segments of regular content and expandable quotes.
func extractExpandableQuotes(text string) []segment {
	matches := reExpQuote.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return []segment{{isQuote: false, content: text}}
	}

	var segments []segment
	lastEnd := 0

	for _, match := range matches {
		// Text before this quote
		if match[0] > lastEnd {
			segments = append(segments, segment{isQuote: false, content: text[lastEnd:match[0]]})
		}

		// Extract the inner content (between sentinels)
		full := text[match[0]:match[1]]
		inner := reExpQuote.FindStringSubmatch(full)[1]
		segments = append(segments, segment{isQuote: true, content: inner})
		lastEnd = match[1]
	}

	// Text after last quote
	if lastEnd < len(text) {
		segments = append(segments, segment{isQuote: false, content: text[lastEnd:]})
	}

	return segments
}

// renderExpandableQuote formats text as a Telegram expandable blockquote.
func renderExpandableQuote(content string) string {
	// Truncate at 3800 chars to stay within Telegram limits
	if len(content) > 3800 {
		content = content[:3800] + "\n... (truncated)"
	}

	escaped := escapeMarkdownV2(content)
	lines := strings.Split(escaped, "\n")
	var quoted []string
	for i, line := range lines {
		if i == len(lines)-1 {
			quoted = append(quoted, ">"+line+"||")
		} else {
			quoted = append(quoted, ">"+line)
		}
	}
	return strings.Join(quoted, "\n")
}

// convertWithGoldmark parses text as CommonMark and renders it with the appropriate renderer.
// A fresh goldmark instance is created per call (cheap, enables mutable renderer state).
func convertWithGoldmark(text string, plain bool) string {
	if text == "" {
		return ""
	}

	var nodeRenderer renderer.NodeRenderer
	if plain {
		nodeRenderer = newPlainRenderer()
	} else {
		nodeRenderer = newTelegramRenderer()
	}

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRenderer(
			renderer.NewRenderer(
				renderer.WithNodeRenderers(
					// Priority 100: must beat GFM's HTML renderers (priority 500)
					util.Prioritized(nodeRenderer, 100),
				),
			),
		),
	)

	var buf bytes.Buffer
	if err := md.Convert([]byte(text), &buf); err != nil {
		// Fallback: escape as plain text
		if plain {
			return text
		}
		return escapeMarkdownV2(text)
	}

	result := buf.String()

	// Trim trailing newlines (goldmark always adds trailing paragraph newlines)
	result = strings.TrimRight(result, "\n")

	return result
}

// escapeMarkdownV2 escapes all MarkdownV2 special characters.
func escapeMarkdownV2(text string) string {
	var b strings.Builder
	b.Grow(len(text) * 2)
	for _, r := range text {
		if strings.ContainsRune(mdv2Special, r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// escapeCodeContent escapes content inside code blocks (only ` and \ need escaping).
func escapeCodeContent(text string) string {
	text = strings.ReplaceAll(text, "\\", "\\\\")
	text = strings.ReplaceAll(text, "`", "\\`")
	return text
}

// escapeURL escapes special characters in URLs for MarkdownV2.
func escapeURL(url string) string {
	url = strings.ReplaceAll(url, "\\", "\\\\")
	url = strings.ReplaceAll(url, ")", "\\)")
	return url
}
