package main

import (
	"fmt"
	"os"

	"golang.org/x/term"

	"github.com/neerajg/gig"
)

// ANSI color codes.
const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	dim     = "\033[2m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	blue    = "\033[34m"
	magenta = "\033[35m"
	cyan    = "\033[36m"
	white   = "\033[37m"
)

// colorEnabled reports whether the terminal supports color output.
// Disabled when stdout is not a terminal (piped/redirected) or NO_COLOR is set.
func colorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func colorize(color, s string) string {
	if !colorEnabled() {
		return s
	}
	return color + s + reset
}

// colorStatus returns the status icon with appropriate color.
func colorStatus(s gig.Status) string {
	icon := statusIcon(s)
	if !colorEnabled() {
		return "[" + icon + "]"
	}
	switch s {
	case gig.StatusOpen:
		return "[" + icon + "]"
	case gig.StatusInProgress:
		return colorize(blue+bold, "["+icon+"]")
	case gig.StatusBlocked:
		return colorize(red+bold, "["+icon+"]")
	case gig.StatusDeferred:
		return colorize(yellow, "["+icon+"]")
	case gig.StatusClosed:
		return colorize(green, "["+icon+"]")
	default:
		return "[" + icon + "]"
	}
}

// colorPriority returns the priority label with color.
func colorPriority(p gig.Priority) string {
	label := fmt.Sprintf("P%d", p)
	if !colorEnabled() {
		return label
	}
	switch p {
	case gig.P0:
		return colorize(red+bold, label)
	case gig.P1:
		return colorize(yellow+bold, label)
	case gig.P2:
		return label
	case gig.P3:
		return colorize(dim, label)
	case gig.P4:
		return colorize(dim, label)
	default:
		return label
	}
}

// colorAssignee returns the assignee with color.
func colorAssignee(assignee string) string {
	if assignee == "" {
		return ""
	}
	return colorize(cyan, "@"+assignee)
}

// colorID returns the task ID with color.
func colorID(id string) string {
	return colorize(dim, id)
}
