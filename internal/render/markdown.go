package render

import (
	"regexp"
	"strings"
)

// MarkdownV2 special characters that must be escaped outside formatted regions.
const mdv2Special = `_*[]()~` + "`" + `>#+-=|{}.!`

var (
	reCodeBlock  = regexp.MustCompile("(?s)```([\\s\\S]*?)```")
	reInlineCode = regexp.MustCompile("`([^`]+)`")
	reBold       = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reItalic     = regexp.MustCompile(`(?:^|[^*])_(.+?)_`)
	reLink       = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reExpQuote   = regexp.MustCompile(regexp.QuoteMeta(ExpQuoteStart) + `([\s\S]*?)` + regexp.QuoteMeta(ExpQuoteEnd))
)

// ToMarkdownV2 converts standard Markdown to Telegram MarkdownV2 format.
func ToMarkdownV2(text string) string {
	// Protect code blocks and inline code by replacing with placeholders
	var codeBlocks []string
	var inlineCodes []string

	// Replace code blocks
	result := reCodeBlock.ReplaceAllStringFunc(text, func(match string) string {
		idx := len(codeBlocks)
		// Extract content between ```
		inner := match[3 : len(match)-3]
		// In MarkdownV2 code blocks, only ` and \ need escaping
		escaped := escapeCodeContent(inner)
		codeBlocks = append(codeBlocks, "```"+escaped+"```")
		return placeholderBlock(idx)
	})

	// Replace inline code
	result = reInlineCode.ReplaceAllStringFunc(result, func(match string) string {
		idx := len(inlineCodes)
		inner := match[1 : len(match)-1]
		escaped := escapeCodeContent(inner)
		inlineCodes = append(inlineCodes, "`"+escaped+"`")
		return placeholderInline(idx)
	})

	// Process expandable quotes — store as placeholders to avoid re-escaping
	var expQuotes []string
	result = reExpQuote.ReplaceAllStringFunc(result, func(match string) string {
		idx := len(expQuotes)
		inner := reExpQuote.FindStringSubmatch(match)[1]
		// Escape the inner content
		escaped := escapeMarkdownV2(inner)
		// Format as Telegram expandable blockquote
		lines := strings.Split(escaped, "\n")
		var quoted []string
		for i, line := range lines {
			if i == len(lines)-1 {
				quoted = append(quoted, ">"+line+"||")
			} else {
				quoted = append(quoted, ">"+line)
			}
		}
		expQuotes = append(expQuotes, strings.Join(quoted, "\n"))
		return placeholderQuote(idx)
	})

	// Convert bold: **text** → *text*
	result = reBold.ReplaceAllStringFunc(result, func(match string) string {
		inner := match[2 : len(match)-2]
		escaped := escapeMarkdownV2(inner)
		return "*" + escaped + "*"
	})

	// Convert links: [text](url) → [text](url) with escaping
	result = reLink.ReplaceAllStringFunc(result, func(match string) string {
		parts := reLink.FindStringSubmatch(match)
		linkText := escapeMarkdownV2(parts[1])
		linkURL := escapeURL(parts[2])
		return "[" + linkText + "](" + linkURL + ")"
	})

	// Escape remaining special characters (outside of already-processed regions)
	result = escapeMarkdownV2Selective(result)

	// Restore placeholders
	for i, block := range codeBlocks {
		result = strings.Replace(result, placeholderBlock(i), block, 1)
	}
	for i, code := range inlineCodes {
		result = strings.Replace(result, placeholderInline(i), code, 1)
	}
	for i, quote := range expQuotes {
		result = strings.Replace(result, placeholderQuote(i), quote, 1)
	}

	return result
}

// ToPlainText strips all markdown formatting and returns raw text.
func ToPlainText(text string) string {
	result := text

	// Remove expandable quote markers
	result = strings.ReplaceAll(result, ExpQuoteStart, "")
	result = strings.ReplaceAll(result, ExpQuoteEnd, "")

	// Remove code block markers
	result = reCodeBlock.ReplaceAllString(result, "$1")

	// Remove inline code markers
	result = reInlineCode.ReplaceAllString(result, "$1")

	// Remove bold
	result = reBold.ReplaceAllString(result, "$1")

	// Convert links to "text (url)"
	result = reLink.ReplaceAllString(result, "$1 ($2)")

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

// escapeMarkdownV2Selective escapes special chars while preserving already-formatted regions.
// This is a simplified approach — it escapes chars that aren't part of MarkdownV2 syntax we've already handled.
func escapeMarkdownV2Selective(text string) string {
	// Characters to escape outside formatted regions
	// We DON'T escape *, `, [, ], (, ) since those are part of formatting we've applied
	escapeChars := `~>#+-=|{}.!`

	var b strings.Builder
	b.Grow(len(text) * 2)

	for i := 0; i < len(text); i++ {
		ch := text[i]

		// Skip placeholders
		if ch == '\x00' {
			b.WriteByte(ch)
			continue
		}

		// Already escaped character
		if ch == '\\' && i+1 < len(text) {
			b.WriteByte(ch)
			i++
			b.WriteByte(text[i])
			continue
		}

		if strings.ContainsRune(escapeChars, rune(ch)) {
			b.WriteByte('\\')
		}
		b.WriteByte(ch)
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

func placeholderBlock(idx int) string {
	return "\x00CODEBLOCK" + string(rune('0'+idx)) + "\x00"
}

func placeholderInline(idx int) string {
	return "\x00INLINE" + string(rune('0'+idx)) + "\x00"
}

func placeholderQuote(idx int) string {
	return "\x00QUOTE" + string(rune('0'+idx)) + "\x00"
}
