package cmd

import (
	"bytes"
	"io"
	"regexp"
	"strings"

	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
)

var (
	// helpHeadingStyle styles the section labels (NAME:, USAGE:, ...).
	helpHeadingStyle = color.New(color.Bold, color.FgHiCyan)
	// helpNameStyle styles command and flag names in list entries.
	helpNameStyle = color.New(color.Bold, color.FgHiGreen)
	// helpArgStyle mutes the argument placeholders in usage lines.
	helpArgStyle = color.New(color.Faint)
)

// helpLabels are the section labels the urfave/cli help templates emit on a
// line of their own.
var helpLabels = map[string]bool{
	"NAME:":           true,
	"USAGE:":          true,
	"VERSION:":        true,
	"DESCRIPTION:":    true,
	"CATEGORY:":       true,
	"COMMANDS:":       true,
	"OPTIONS:":        true,
	"GLOBAL OPTIONS:": true,
	"AUTHOR:":         true,
	"AUTHORS:":        true,
	"COPYRIGHT:":      true,
}

// helpArgPattern matches the <placeholder> and [optional] tokens of usage
// lines.
var helpArgPattern = regexp.MustCompile(`<[^>]+>|\[[^]]+\]`)

// colorHelp makes the default help printer style its output.
//
// Styling has to happen after urfave/cli renders the help: it lays the text
// out with a tabwriter, and escape codes inserted before that would count
// towards column widths and break the alignment.
func colorHelp() {
	cli.HelpPrinter = printHelp
}

// printHelp renders the default help, styles it, and writes it out.
func printHelp(w io.Writer, tmpl string, data any) {
	var buf bytes.Buffer
	cli.DefaultPrintHelp(&buf, tmpl, data)

	_, _ = io.WriteString(w, colorizeHelp(buf.String()))
}

// colorizeHelp styles fully rendered help text. It tracks the current section
// so that each kind of line gets the right treatment.
func colorizeHelp(help string) string {
	lines := strings.Split(help, "\n")

	section := ""
	for i, line := range lines {
		label := strings.TrimSpace(line)
		if helpLabels[label] {
			section = label
			lines[i] = helpHeadingStyle.Sprint(label)

			continue
		}

		switch section {
		case "NAME:":
			lines[i] = styleNameLine(line)
		case "COMMANDS:", "OPTIONS:", "GLOBAL OPTIONS:":
			lines[i] = styleListLine(line)
		case "USAGE:":
			lines[i] = helpArgPattern.ReplaceAllStringFunc(line, func(m string) string {
				return helpArgStyle.Sprint(m)
			})
		}
	}

	return strings.Join(lines, "\n")
}

// styleNameLine styles the command path on the "NAME:" line, e.g.
// "   oir install - Install a tool".
func styleNameLine(line string) string {
	name, usage, ok := strings.Cut(line, " - ")
	if !ok {
		return line
	}

	return helpNameStyle.Sprint(name) + " - " + usage
}

// styleListLine styles the first column of a command or flag entry, e.g.
// "   install, i  Install a tool".
func styleListLine(line string) string {
	indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
	body := line[len(indent):]

	name, desc, ok := splitColumns(body)
	if !ok {
		return line
	}

	return indent + helpNameStyle.Sprint(name) + desc
}

// splitColumns splits a tabwriter row into its first cell and the padding plus
// everything after it. It finds the first run of two or more spaces, which is
// the column gap the help printer writes.
func splitColumns(body string) (name, rest string, ok bool) {
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
