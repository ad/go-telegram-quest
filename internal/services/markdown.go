package services

import (
	"fmt"
	"strings"

	"github.com/go-telegram/bot"
)

func EscapeUserContent(text string) string {
	return bot.EscapeMarkdownUnescaped(text)
}

func FormatBold(text string) string {
	return fmt.Sprintf("*%s*", EscapeUserContent(text))
}

func FormatItalic(text string) string {
	return fmt.Sprintf("_%s_", EscapeUserContent(text))
}

func FormatCode(text string) string {
	return fmt.Sprintf("`%s`", EscapeUserContent(text))
}

func FormatLink(text, url string) string {
	return fmt.Sprintf("[%s](%s)", EscapeUserContent(text), EscapeUserContent(url))
}

func SafeConcat(parts ...string) string {
	var sb strings.Builder
	for _, part := range parts {
		sb.WriteString(part)
	}
	return sb.String()
}
