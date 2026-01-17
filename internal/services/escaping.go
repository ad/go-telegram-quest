package services

import (
	"fmt"
	"html"
	"strings"
)

func FormatBold(text string) string {
	return fmt.Sprintf("<b>%s</b>", html.EscapeString(text))
}

func FormatItalic(text string) string {
	return fmt.Sprintf("<i>%s</i>", html.EscapeString(text))
}

func FormatCode(text string) string {
	return fmt.Sprintf("<pre>%s</pre>", html.EscapeString(text))
}

func FormatLink(text, url string) string {
	return fmt.Sprintf("<a href=\"%s\">%s</a>", html.EscapeString(url), html.EscapeString(text))
}

func SafeConcat(parts ...string) string {
	var sb strings.Builder
	for _, part := range parts {
		sb.WriteString(part)
	}
	return sb.String()
}
