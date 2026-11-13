// Package cli is the command tree of `photos` and its dispatch.
//
// Hand-rolled on flag.FlagSet rather than built on a framework: one static
// binary whose only dependency is the Photos.sh Go library, itself standard
// library only. What that costs is help text and completion, written here
// once.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Command is one verb, or a noun holding verbs. It has either Run or
// Children, never both.
type Command struct {
	// Name is the word this is invoked as.
	Name string
	// Aliases are other spellings.
	Aliases []string
	// Group is the heading it is listed under in the root's help.
	Group string
	// Summary is the one line in the parent's help: lower case, no full
	// stop, starts with a verb.
	Summary string
	// Args is what follows the command's name in its usage line:
	// "[flags] <image>...".
	Args string
	// Long is printed under the usage line.
	Long string
	// Examples are real invocations, one per line, without a prompt. A line
	// starting with "#" is a comment.
	Examples []string
	// Flags registers this command's own flags. It may be called more than
	// once (for help and completion as well as for parsing).
	Flags func(fs *flag.FlagSet)
	// Run does the work. args are what is left after the flags.
	Run func(ctx context.Context, env *Env, args []string) error
	// Children are subcommands, for a noun.
	Children []*Command
}

// lookup finds a child by name or alias.
func (c *Command) lookup(name string) *Command {
	for _, child := range c.Children {
		if child.Name == name {
			return child
		}
		for _, alias := range child.Aliases {
			if alias == name {
				return child
			}
		}
	}
	return nil
}

// childNames is every spelling of every child, sorted.
func (c *Command) childNames() []string {
	var names []string
	for _, child := range c.Children {
		names = append(names, child.Name)
		names = append(names, child.Aliases...)
	}
	sort.Strings(names)
	return names
}

// UsageError is a command invoked wrongly: an unknown command or flag, a
// missing argument, options that contradict each other. It is the difference
// between exit code 2 and the others.
type UsageError struct {
	// Path is the command that was misused, as words: {"photos", "resize"}.
	Path []string
	Err  error
}

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

// usagef builds a UsageError; Execute fills in the command.
func usagef(format string, args ...any) error {
	return &UsageError{Err: fmt.Errorf(format, args...)}
}

// Execute walks the command tree and runs what the arguments name.
func Execute(ctx context.Context, root *Command, env *Env, args []string) error {
	path := []string{root.Name}

	// Global flags may come before the command: `photos --json account`.
	lead := flag.NewFlagSet(root.Name, flag.ContinueOnError)
	lead.SetOutput(io.Discard)
	lead.Usage = func() {}
	help := lead.Bool("help", false, "")
	lead.BoolVar(help, "h", false, "")
	version := lead.Bool("version", false, "")
	env.registerGlobals(lead)
	if err := lead.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			*help = true
		} else {
			return &UsageError{Path: path, Err: flagError(lead, err)}
		}
	}
	args = lead.Args()
	if *version {
		fmt.Fprintln(env.Out, env.Version)
		return nil
	}

	cmd := root
	for len(cmd.Children) > 0 && len(args) > 0 {
		if strings.HasPrefix(args[0], "-") {
			break
		}
		child := cmd.lookup(args[0])
		if child == nil {
			err := fmt.Errorf("unknown command %q for %q%s", args[0], strings.Join(path, " "),
				didYouMean(args[0], cmd.childNames(), ""))
			return &UsageError{Path: path, Err: err}
		}
		path = append(path, child.Name)
		cmd = child
		args = args[1:]
	}

	fs := flag.NewFlagSet(strings.Join(path, " "), flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	fs.BoolVar(help, "help", *help, "")
	fs.BoolVar(help, "h", *help, "")
	env.registerGlobals(fs)
	if cmd.Flags != nil {
		cmd.Flags(fs)
	}

	// Flags are moved ahead of the other arguments, because flag.Parse stops
	// at the first argument that is not one, and `photos resize a.jpg
	// --width 100` has to work.
	ordered, err := permute(fs, args)
	if err != nil {
		return &UsageError{Path: path, Err: err}
	}
	if err := fs.Parse(ordered); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			return &UsageError{Path: path, Err: flagError(fs, err)}
		}
		*help = true
	}

	if *help || cmd.Run == nil {
		// A noun on its own is somebody asking what it does: not an error.
		printHelp(env.Out, root, cmd, path)
		return nil
	}

	env.path = path
	if err := env.setup(); err != nil {
		return withPath(err, path)
	}
	return withPath(cmd.Run(ctx, env, fs.Args()), path)
}

// withPath names the command in a usage error raised inside it.
func withPath(err error, path []string) error {
	var usage *UsageError
	if errors.As(err, &usage) && usage.Path == nil {
		usage.Path = path
	}
	return err
}

// flagError turns the flag package's error into ours: an unknown flag is
// named with two dashes and, when one is close, a suggestion.
func flagError(fs *flag.FlagSet, err error) error {
	const unknown = "flag provided but not defined: "
	msg := err.Error()
	if !strings.HasPrefix(msg, unknown) {
		// The flag package names a flag with one dash; here they have two.
		return errors.New(strings.NewReplacer("for flag -", "for flag --", "flag needs an argument: -", "flag needs an argument: --",
			"invalid boolean value", "invalid value", " for -", " for flag --").Replace(msg))
	}
	name := strings.TrimLeft(strings.TrimPrefix(msg, unknown), "-")
	var known []string
	fs.VisitAll(func(f *flag.Flag) {
		if len(f.Name) > 1 {
			known = append(known, f.Name)
		}
	})
	return fmt.Errorf("unknown flag --%s%s", name, didYouMean(name, known, "--"))
}

// didYouMean is " (did you mean X?)" for the candidate closest to word, or
// nothing when none is close.
func didYouMean(word string, candidates []string, prefix string) string {
	best, bestDistance := "", 1<<30
	for _, c := range candidates {
		d := distance(word, c)
		if strings.HasPrefix(c, word) && len(word) >= 2 {
			d = 1 // an unfinished word is close to what it starts
		}
		if d < bestDistance {
			best, bestDistance = c, d
		}
	}
	limit := 2
	if len(word) > 6 {
		limit = 3
	}
	if best == "" || bestDistance > limit || bestDistance >= len(word) {
		// Not a slip of the finger; perhaps a shortening: "remove-bg".
		best = ""
		longest := max(3, len(word)*2/3)
		for _, c := range candidates {
			if n := commonPrefix(word, c); n > longest {
				best, longest = c, n
			}
		}
		if best == "" {
			return ""
		}
	}
	return fmt.Sprintf(" (did you mean %s%s?)", prefix, best)
}

// commonPrefix is how many leading bytes two words share.
func commonPrefix(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

// distance is the edit distance between two words.
func distance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

// permute reorders arguments so that flags come first. A flag that takes a
// value carries the next argument with it. "-" alone is standard input, not
// a flag, and everything after a bare "--" is an argument whatever it looks
// like.
func permute(fs *flag.FlagSet, args []string) ([]string, error) {
	var flags, positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if len(arg) < 2 || arg[0] != '-' {
			positionals = append(positionals, arg)
			continue
		}
		flags = append(flags, arg)
		if strings.Contains(arg, "=") {
			continue
		}
		formal := fs.Lookup(strings.TrimLeft(arg, "-"))
		if formal == nil || isBoolFlag(formal) {
			continue
		}
		if i+1 >= len(args) {
			return nil, fmt.Errorf("flag needs an argument: --%s", strings.TrimLeft(arg, "-"))
		}
		i++
		flags = append(flags, args[i])
	}
	out := append(flags, "--")
	return append(out, positionals...), nil
}

func isBoolFlag(f *flag.Flag) bool {
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

// shortPrefix marks, in a flag's usage text, a one-letter spelling of
// another flag; see short.
const shortPrefix = "short for --"

// short registers a one-letter spelling of a flag already registered.
func short(fs *flag.FlagSet, letter, long string) {
	fs.Var(fs.Lookup(long).Value, letter, shortPrefix+long)
}

// flagInfo describes one flag for help, USAGE.md and completion.
type flagInfo struct {
	Name        string // without dashes
	Short       string // a one-letter spelling, or ""
	Placeholder string // the name of its value; "" for a boolean
	Usage       string
	Default     string // "" when the default is the zero value
}

// label is the flag as it is typed: "-o, --output file".
func (f flagInfo) label() string {
	s := "--" + f.Name
	if f.Short != "" {
		s = "-" + f.Short + ", " + s
	}
	if f.Placeholder != "" {
		s += " " + f.Placeholder
	}
	return s
}

// describeFlags reads back what a registrar registers, so help, USAGE.md and
// completion cannot disagree with what is parsed.
func describeFlags(register func(fs *flag.FlagSet)) []flagInfo {
	if register == nil {
		return nil
	}
	fs := flag.NewFlagSet("describe", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	register(fs)

	shorts := map[string]string{}
	fs.VisitAll(func(f *flag.Flag) {
		if long, ok := strings.CutPrefix(f.Usage, shortPrefix); ok {
			shorts[long] = f.Name
		}
	})
	var out []flagInfo
	fs.VisitAll(func(f *flag.Flag) {
		if strings.HasPrefix(f.Usage, shortPrefix) {
			return
		}
		placeholder, usage := flag.UnquoteUsage(f)
		info := flagInfo{Name: f.Name, Short: shorts[f.Name], Placeholder: placeholder, Usage: usage}
		switch f.DefValue {
		case "", "0", "false", "0s":
		default:
			info.Default = f.DefValue
		}
		out = append(out, info)
	})
	return out
}

// commandFlags is a command's own flags; globalFlags the ones every command
// takes.
func commandFlags(cmd *Command) []flagInfo { return describeFlags(cmd.Flags) }

func globalFlags() []flagInfo {
	flags := describeFlags((&Env{}).registerGlobals)
	return append(flags, flagInfo{Name: "help", Short: "h", Usage: "show help for the command"})
}

// usageLine is the command as it is typed, with its arguments.
func usageLine(cmd *Command, path []string) string {
	line := strings.Join(path, " ")
	switch {
	case cmd.Args != "":
		line += " " + cmd.Args
	case len(cmd.Children) > 0:
		line += " <command> [flags]"
	}
	return line
}

// printHelp writes a command's help.
func printHelp(w io.Writer, root, cmd *Command, path []string) {
	if cmd.Summary != "" {
		fmt.Fprintf(w, "%s%s\n\n", strings.ToUpper(cmd.Summary[:1]), cmd.Summary[1:])
	}
	fmt.Fprintf(w, "Usage:\n  %s\n", usageLine(cmd, path))
	if len(cmd.Aliases) > 0 {
		fmt.Fprintf(w, "\nAlias: %s\n", strings.Join(cmd.Aliases, ", "))
	}
	if cmd.Long != "" {
		fmt.Fprintf(w, "\n%s\n", strings.TrimSpace(cmd.Long))
	}

	if len(cmd.Children) > 0 {
		width := 0
		for _, child := range cmd.Children {
			width = max(width, len(child.Name))
		}
		for _, group := range groupsOf(cmd) {
			heading := group
			if heading == "" {
				heading = "Commands"
			}
			fmt.Fprintf(w, "\n%s:\n", heading)
			for _, child := range cmd.Children {
				if child.Group == group {
					fmt.Fprintf(w, "  %-*s  %s\n", width, child.Name, child.Summary)
				}
			}
		}
	}

	if len(cmd.Examples) > 0 {
		fmt.Fprintf(w, "\nExamples:\n")
		for _, example := range cmd.Examples {
			fmt.Fprintf(w, "  %s\n", example)
		}
	}

	printFlags(w, "Flags", commandFlags(cmd))
	printFlags(w, "Global flags", globalFlags())

	if len(cmd.Children) > 0 {
		fmt.Fprintf(w, "\nRun `%s help <command>` for one command.\n", root.Name)
	}
}

// groupsOf is the groups of a command's children, in order of appearance.
func groupsOf(cmd *Command) []string {
	var groups []string
	seen := map[string]bool{}
	for _, child := range cmd.Children {
		if !seen[child.Group] {
			seen[child.Group] = true
			groups = append(groups, child.Group)
		}
	}
	return groups
}

func printFlags(w io.Writer, heading string, flags []flagInfo) {
	if len(flags) == 0 {
		return
	}
	width := 0
	for _, f := range flags {
		width = max(width, len(flagColumn(f)))
	}
	fmt.Fprintf(w, "\n%s:\n", heading)
	for _, f := range flags {
		usage := f.Usage
		if f.Default != "" {
			usage += fmt.Sprintf(" (default %s)", f.Default)
		}
		fmt.Fprintf(w, "  %-*s  %s\n", width, flagColumn(f), usage)
	}
}

// flagColumn lines the long names up whether or not there is a short one.
func flagColumn(f flagInfo) string {
	if f.Short != "" {
		return f.label()
	}
	return "    " + f.label()
}

// find walks words down the tree: {"jobs", "create"}.
func find(root *Command, words []string) (*Command, []string, error) {
	cmd, path := root, []string{root.Name}
	for _, word := range words {
		child := cmd.lookup(word)
		if child == nil {
			return nil, path, fmt.Errorf("unknown command %q for %q%s", word, strings.Join(path, " "),
				didYouMean(word, cmd.childNames(), ""))
		}
		cmd, path = child, append(path, child.Name)
	}
	return cmd, path, nil
}

// Flag values the flag package does not have.

// stringList is a flag that may be given several times.
type stringList []string

func (l *stringList) String() string     { return strings.Join(*l, ",") }
func (l *stringList) Set(v string) error { *l = append(*l, v); return nil }

// optionalString is a string flag that knows whether it was given, for one
// whose empty value means something.
type optionalString struct {
	value string
	set   bool
}

func (o *optionalString) String() string     { return o.value }
func (o *optionalString) Set(v string) error { o.value, o.set = v, true; return nil }

// The flags of options whose zero is a value of its own (a margin of 0, a
// tolerance of 0, an automatic correction turned off). The library has them
// as pointers: nil is not sent, and the API uses its default. These leave
// the pointer nil unless the flag is given.

// What the flag package says after "invalid value … for flag --name: ".
var (
	errNotANumber  = errors.New("not a number")
	errNotWholeNum = errors.New("not a whole number")
	errNotBoolean  = errors.New("want true or false")
)

// optionalInt is a whole-number flag that knows whether it was given.
type optionalInt struct{ to **int }

func (o optionalInt) String() string {
	if o.to == nil || *o.to == nil {
		return ""
	}
	return strconv.Itoa(**o.to)
}

func (o optionalInt) Set(v string) error {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return errNotWholeNum
	}
	*o.to = &n
	return nil
}

// optionalFloat is a number flag that knows whether it was given.
type optionalFloat struct{ to **float64 }

func (o optionalFloat) String() string {
	if o.to == nil || *o.to == nil {
		return ""
	}
	return strconv.FormatFloat(**o.to, 'f', -1, 64)
}

func (o optionalFloat) Set(v string) error {
	f, err := parseNumber(v)
	if err != nil {
		return errNotANumber
	}
	*o.to = &f
	return nil
}

// optionalBool is a boolean flag that knows whether it was given, for a
// switch whose "off" is not the same as saying nothing: --levels turns it
// on, --levels=false turns it off.
type optionalBool struct{ to **bool }

func (o optionalBool) String() string {
	if o.to == nil || *o.to == nil {
		return ""
	}
	return strconv.FormatBool(**o.to)
}

func (o optionalBool) Set(v string) error {
	b, err := strconv.ParseBool(v)
	if err != nil {
		return errNotBoolean
	}
	*o.to = &b
	return nil
}

func (o optionalBool) IsBoolFlag() bool { return true }

// parseNumber reads a plain number: no NaN, no infinity.
func parseNumber(s string) (float64, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, errNotANumber
	}
	return f, nil
}
