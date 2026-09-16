package term

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

func PrintFirstHeader(w io.Writer, description string) error {
	return printHeader(w, description, "")
}

func PrintNthHeader(w io.Writer, description string) error {
	return printHeader(w, description, "\n")
}

func printHeader(w io.Writer, description string, prefix string) error {
	header := Header(description, IsTTY(w))
	if header == "" {
		return nil
	}

	_, err := fmt.Fprintf(w, "%s%s\n", prefix, header)
	return err
}

func Header(description string, isTTY bool) string {
	if description == "" {
		return ""
	}

	const totalWidth = 60
	prefix := "── "
	suffix := " "

	descriptionWidth := visibleWidth(description)
	barWidth := max(totalWidth-visibleWidth(prefix)-descriptionWidth-visibleWidth(suffix), 0)
	bar := suffix + strings.Repeat("─", barWidth)
	if !isTTY {
		return prefix + description + bar
	}
	return Color(Dim, prefix) + description + Color(Dim, bar)
}

func visibleWidth(text string) int {
	width := 0
	for len(text) > 0 {
		if strings.HasPrefix(text, "\033[") {
			for index := 2; index < len(text); index++ {
				if text[index] >= 0x40 && text[index] <= 0x7e {
					text = text[index+1:]
					break
				}
			}
			if strings.HasPrefix(text, "\033[") {
				text = text[1:]
			}
			continue
		}
		_, size := utf8.DecodeRuneInString(text)
		width++
		text = text[size:]
	}
	return width
}
