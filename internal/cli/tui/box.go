package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/lipgloss"
)

// Box prints content inside a rounded branded box.
func Box(w io.Writer, title, content string) {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorViolet).
		Padding(0, 1).
		Width(bannerWidth)

	header := BrandStyle.Render(title)
	body := fmt.Sprintf("%s\n%s", header, content)

	_, _ = fmt.Fprintln(w, style.Render(body))
}

// InfoBox prints content inside a blue info box.
func InfoBox(w io.Writer, title, content string) {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorInfo).
		Padding(0, 1).
		Width(bannerWidth)

	header := InfoLabelStyle.Render(title)
	body := fmt.Sprintf("%s\n%s", header, content)

	fmt.Fprintln(w, style.Render(body))
}

// ErrorBox prints content inside a red error box.
func ErrorBox(w io.Writer, title, content string) {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorError).
		Padding(0, 1).
		Width(bannerWidth)

	header := ErrorLabelStyle.Render(title)
	body := fmt.Sprintf("%s\n%s", header, content)

	fmt.Fprintln(w, style.Render(body))
}

// ResultBox prints a summary result box (e.g., doctor diagnosis).
func ResultBox(w io.Writer, passed bool, message string) {
	if passed {
		resultBox(w, ColorSuccess, SuccessLabelStyle.Render("✓"), SuccessLabelStyle.Render("PASS"), message)
		return
	}
	resultBox(w, ColorError, ErrorLabelStyle.Render("✗"), ErrorLabelStyle.Render("FAIL"), message)
}

// WarnResultBox prints the summary box for a report whose lines hold
// warnings but no failure.
func WarnResultBox(w io.Writer, message string) {
	resultBox(w, ColorWarning, WarningLabelStyle.Render("⚠"), WarningLabelStyle.Render("WARN"), message)
}

func resultBox(w io.Writer, border lipgloss.Color, icon, label, message string) {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Width(bannerWidth)

	body := fmt.Sprintf("%s %s  %s", icon, label, message)
	fmt.Fprintln(w, style.Render(body))
}
