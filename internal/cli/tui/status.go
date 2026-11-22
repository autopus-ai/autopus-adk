package tui

import (
	"fmt"
	"io"
)

// Status icons
const (
	iconSuccess = "✓"
	iconError   = "✗"
	iconWarning = "⚠"
	iconInfo    = "ℹ"
	iconStep    = "→"
	iconBullet  = "•"
)

// Success prints a green success message.
func Success(w io.Writer, msg string) {
	icon := SuccessLabelStyle.Render(iconSuccess)
	fmt.Fprintf(w, "  %s %s\n", icon, msg)
}

// Successf prints a formatted green success message.
func Successf(w io.Writer, format string, a ...any) {
	Success(w, fmt.Sprintf(format, a...))
}

// Error prints a red error message.
func Error(w io.Writer, msg string) {
	icon := ErrorLabelStyle.Render(iconError)
	text := ErrorStyle.Render(msg)
	fmt.Fprintf(w, "  %s %s\n", icon, text)
}

// Errorf prints a formatted red error message.
func Errorf(w io.Writer, format string, a ...any) {
	Error(w, fmt.Sprintf(format, a...))
}

// Warn prints a yellow warning message.
func Warn(w io.Writer, msg string) {
	icon := WarningLabelStyle.Render(iconWarning)
	text := WarningStyle.Render(msg)
	fmt.Fprintf(w, "  %s %s\n", icon, text)
}

// Warnf prints a formatted yellow warning message.
func Warnf(w io.Writer, format string, a ...any) {
	Warn(w, fmt.Sprintf(format, a...))
}

// Info prints a blue info message.
func Info(w io.Writer, msg string) {
	icon := InfoLabelStyle.Render(iconInfo)
	fmt.Fprintf(w, "  %s %s\n", icon, msg)
}

// Infof prints a formatted blue info message.
func Infof(w io.Writer, format string, a ...any) {
	Info(w, fmt.Sprintf(format, a...))
}

// Step prints a progress step indicator.
func Step(w io.Writer, current, total int, msg string) {
	icon := BrandStyle.Render(iconStep)
	counter := MutedStyle.Render(fmt.Sprintf("[%d/%d]", current, total))
	fmt.Fprintf(w, "  %s %s %s\n", icon, counter, msg)
}

// Bullet prints a bulleted item.
func Bullet(w io.Writer, msg string) {
	icon := AccentStyle.Render(iconBullet)
	fmt.Fprintf(w, "    %s %s\n", icon, msg)
}

// Tag prints a styled inline tag (e.g., mode, platform name).
func Tag(label, value string) string {
	l := MutedStyle.Render(label + ":")
	v := BrandStyle.Render(value)
	return fmt.Sprintf("%s %s", l, v)
}

// OK prints a compact OK status for checklist items.
func OK(w io.Writer, msg string) {
	label := SuccessLabelStyle.Render("OK")
	fmt.Fprintf(w, "  [%s] %s\n", label, msg)
}

// FAIL prints a compact FAIL status for checklist items.
func FAIL(w io.Writer, msg string) {
	countError(w)
	label := ErrorLabelStyle.Render("ERROR")
	text := ErrorStyle.Render(msg)
	fmt.Fprintf(w, "  [%s] %s\n", label, text)
}

// SKIP prints a compact WARN status for checklist items.
func SKIP(w io.Writer, msg string) {
	label := WarningLabelStyle.Render("WARN")
	text := WarningStyle.Render(msg)
	fmt.Fprintf(w, "  [%s] %s\n", label, text)
}
