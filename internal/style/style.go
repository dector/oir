// Package style renders the styled fragments used in oir's help and command
// output.
//
// fatih/color disables itself when the output is not a terminal or when
// NO_COLOR is set, so redirected and piped output stays plain text.
package style

import "github.com/fatih/color"

var (
	heading = color.New(color.Bold, color.FgHiCyan)
	name    = color.New(color.Bold, color.FgHiGreen)
	muted   = color.New(color.Faint)
	success = color.New(color.FgHiGreen)
	warn    = color.New(color.FgHiYellow)
	failure = color.New(color.FgHiRed)
)

// Heading styles a section label such as "USAGE:".
func Heading(s string) string { return heading.Sprint(s) }

// Name styles an identifier the user can act on: a command, a flag, a tool
// spec, or an alias key.
func Name(s string) string { return name.Sprint(s) }

// Muted styles secondary detail, such as an argument placeholder or a field
// label.
func Muted(s string) string { return muted.Sprint(s) }

// Success styles a positive outcome, such as "installed".
func Success(s string) string { return success.Sprint(s) }

// Warn styles a warning prefix.
func Warn(s string) string { return warn.Sprint(s) }

// Error styles a failure prefix or message.
func Error(s string) string { return failure.Sprint(s) }

// SplitColumns splits a tabwriter row into its first cell and the padding plus
// everything that follows it. It looks for the first run of two or more
// spaces, which is the column gap the help printer and tabwriter use.
//
// Style the cells after layout, never before: escape codes count towards
// column widths and would break the alignment.
func SplitColumns(body string) (name, rest string, ok bool) {
	for i := 1; i < len(body); i++ {
		if body[i-1] == ' ' || body[i] != ' ' {
			continue
		}

		j := i
		for j < len(body) && body[j] == ' ' {
			j++
		}
		if j-i < 2 || j == len(body) {
			continue
		}

		return body[:i], body[i:], true
	}

	return "", "", false
}
