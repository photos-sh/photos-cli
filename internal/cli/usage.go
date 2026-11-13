package cli

import (
	"fmt"
	"strings"
)

// exitCodes documents the exit codes, for USAGE.md.
var exitCodes = []struct {
	Code    int
	Meaning string
}{
	{ExitOK, "Everything asked for was done."},
	{ExitFailed, "A file or the request failed: a bad parameter, a broken image, a file in the way, a job item that failed, a webhook that does not verify."},
	{ExitUsage, "The command was wrong: an unknown command or flag, a missing argument. Nothing was sent."},
	{ExitAuth, "Not signed in, or the API refused the key."},
	{ExitPayment, "Out of credits, or the account is suspended."},
	{ExitUnavailable, "The service is busy or could not be reached, after the retries; or a request timed out."},
	{ExitInterrupted, "Interrupted (Ctrl-C)."},
}

// environment documents the environment variables, for USAGE.md.
var environment = []struct{ Name, Meaning string }{
	{EnvAPIKey, "The API key. Wins over the key saved by `photos login`."},
	{EnvBaseURL, "The API to talk to; `--base-url` wins over it. Default `https://api.photos.sh`."},
	{"PHOTOS_CONFIG_DIR", "The directory `photos login` saves the key in, instead of the user's configuration directory."},
	{"NO_COLOR", "Set to anything: no colour, also on a terminal."},
}

// UsageMarkdown is USAGE.md: every command and every flag, generated from
// the command tree so that it cannot drift from what the binary does. A test
// fails when the file in the repository differs; `make usage` rewrites it.
func UsageMarkdown() string {
	root := Root()
	var b strings.Builder

	b.WriteString(`# Using photos

<!-- Generated from the command tree by ` + "`make usage`" + `. Do not edit by hand: a test fails when this file and the binary disagree. -->

Every command and every flag of the ` + "`photos`" + ` command-line client. The
[README](README.md) has what it is, how to install it and a tour;
` + "`photos help <command>`" + ` prints the same as the sections below.

`)

	b.WriteString("## Commands\n\n")
	var index func(cmd *Command, path []string, depth int)
	index = func(cmd *Command, path []string, depth int) {
		for _, child := range cmd.Children {
			childPath := append(append([]string{}, path...), child.Name)
			title := strings.Join(childPath, " ")
			fmt.Fprintf(&b, "%s- [`%s`](#%s): %s\n", strings.Repeat("  ", depth), title, anchor(title), escapeProse(child.Summary))
			index(child, childPath, depth+1)
		}
	}
	index(root, []string{root.Name}, 0)

	b.WriteString("\n## Global flags\n\nEvery command takes these, before or after its own arguments.\n\n")
	flagTable(&b, globalFlags())

	b.WriteString("\n## Exit codes\n\nStable: a script may rely on them. With several files, the code is that of the most severe failure (the highest of 1 to 5).\n\n| Code | Meaning |\n|---|---|\n")
	for _, e := range exitCodes {
		fmt.Fprintf(&b, "| `%d` | %s |\n", e.Code, e.Meaning)
	}

	b.WriteString("\n## Environment\n\n| Variable | Meaning |\n|---|---|\n")
	for _, e := range environment {
		fmt.Fprintf(&b, "| `%s` | %s |\n", e.Name, e.Meaning)
	}

	var section func(cmd *Command, path []string)
	section = func(cmd *Command, path []string) {
		title := strings.Join(path, " ")
		fmt.Fprintf(&b, "\n## %s\n\n", title)
		fmt.Fprintf(&b, "%s%s.\n\n", strings.ToUpper(cmd.Summary[:1]), escapeProse(cmd.Summary[1:]))
		fmt.Fprintf(&b, "```\n%s\n```\n", usageLine(cmd, path))
		if len(cmd.Aliases) > 0 {
			fmt.Fprintf(&b, "\nAlias: `%s`.\n", strings.Join(cmd.Aliases, "`, `"))
		}
		if cmd.Long != "" {
			fmt.Fprintf(&b, "\n%s\n", escapeProse(strings.TrimSpace(cmd.Long)))
		}
		if len(cmd.Children) > 0 {
			b.WriteString("\n| Command | |\n|---|---|\n")
			for _, child := range cmd.Children {
				name := title + " " + child.Name
				fmt.Fprintf(&b, "| [`%s`](#%s) | %s |\n", name, anchor(name), escapeProse(child.Summary))
			}
		}
		if flags := commandFlags(cmd); len(flags) > 0 {
			b.WriteString("\n")
			flagTable(&b, flags)
		}
		if len(cmd.Examples) > 0 {
			b.WriteString("\n```sh\n")
			for _, example := range cmd.Examples {
				b.WriteString(example + "\n")
			}
			b.WriteString("```\n")
		}
		for _, child := range cmd.Children {
			section(child, append(append([]string{}, path...), child.Name))
		}
	}
	for _, child := range root.Children {
		section(child, []string{root.Name, child.Name})
	}
	return b.String()
}

// anchor is the fragment GitHub gives a heading.
func anchor(title string) string {
	return strings.ReplaceAll(strings.ToLower(title), " ", "-")
}

func flagTable(b *strings.Builder, flags []flagInfo) {
	b.WriteString("| Flag | |\n|---|---|\n")
	for _, f := range flags {
		usage := f.Usage
		if f.Default != "" {
			usage += fmt.Sprintf(" (default %s)", f.Default)
		}
		fmt.Fprintf(b, "| `%s` | %s |\n", strings.ReplaceAll(f.label(), "|", `\|`), strings.ReplaceAll(escapeProse(usage), "|", `\|`))
	}
}

// escapeProse keeps Markdown from reading "<name>" as an HTML tag: outside
// code (indented blocks and `spans`), "<" is escaped.
func escapeProse(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "    ") {
			continue
		}
		var out strings.Builder
		inCode := false
		for _, r := range line {
			switch {
			case r == '`':
				inCode = !inCode
			case r == '<' && !inCode:
				out.WriteByte('\\')
			}
			out.WriteRune(r)
		}
		lines[i] = out.String()
	}
	return strings.Join(lines, "\n")
}
