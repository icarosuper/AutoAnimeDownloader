package main

import (
	"strings"

	"github.com/urfave/cli/v2"
)

// reorderInterspersedFlags moves flags that appear after positionals to the
// flag-parse window urfave/cli v2 actually reads. Go's flag.FlagSet.Parse
// stops at the first non-flag, so `animes add 21 --magnet URI` would otherwise
// treat --magnet as a leftover arg. Command names stay in place; global flags
// go to the front; each command's flags sit immediately after that command.
// See decisions.md #107.
func reorderInterspersedFlags(app *cli.App, args []string) []string {
	if len(args) < 2 {
		return args
	}

	rootFlags := indexFlags(app.Flags, true)
	for _, name := range []string{"help", "h", "version"} {
		if _, ok := rootFlags[name]; !ok {
			rootFlags[name] = flagSpec{global: true}
		}
	}

	scopes := []cmdScope{{
		index: rootFlags,
		cmds:  commandMap(app.Commands),
	}}
	var positionals []string

	tokens := args[1:]
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok == "--" {
			positionals = append(positionals, tokens[i:]...)
			break
		}
		if isFlagToken(tok) {
			name, hasInline := splitFlag(tok)
			spec, known := lookupFlag(scopes, name)
			target := &scopes[len(scopes)-1]
			if known && spec.global {
				target = &scopes[0]
			}
			target.flags = append(target.flags, tok)
			if known && spec.takesValue && !hasInline && i+1 < len(tokens) {
				next := tokens[i+1]
				if next != "--" && !isFlagToken(next) {
					target.flags = append(target.flags, next)
					i++
				}
			}
			continue
		}
		if len(positionals) == 0 {
			if cmd, ok := scopes[len(scopes)-1].cmds[tok]; ok {
				scopes = append(scopes, cmdScope{
					name:  tok,
					index: indexFlags(cmd.Flags, false),
					cmds:  commandMap(cmd.Subcommands),
				})
				continue
			}
		}
		positionals = append(positionals, tok)
	}

	out := []string{args[0]}
	out = append(out, scopes[0].flags...)
	for _, s := range scopes[1:] {
		out = append(out, s.name)
		out = append(out, s.flags...)
	}
	return append(out, positionals...)
}

type flagSpec struct {
	takesValue bool
	global     bool
}

type cmdScope struct {
	name  string
	flags []string
	index map[string]flagSpec
	cmds  map[string]*cli.Command
}

func indexFlags(flags []cli.Flag, global bool) map[string]flagSpec {
	m := make(map[string]flagSpec, len(flags)*2)
	for _, f := range flags {
		if f == nil {
			continue
		}
		_, isBool := f.(*cli.BoolFlag)
		spec := flagSpec{takesValue: !isBool, global: global}
		for _, name := range f.Names() {
			m[name] = spec
		}
	}
	return m
}

func commandMap(cmds []*cli.Command) map[string]*cli.Command {
	m := make(map[string]*cli.Command, len(cmds)*2)
	for _, c := range cmds {
		if c == nil {
			continue
		}
		for _, name := range c.Names() {
			m[name] = c
		}
	}
	return m
}

func lookupFlag(scopes []cmdScope, name string) (flagSpec, bool) {
	for i := len(scopes) - 1; i >= 0; i-- {
		if spec, ok := scopes[i].index[name]; ok {
			return spec, true
		}
	}
	return flagSpec{}, false
}

func isFlagToken(s string) bool {
	return len(s) > 1 && s[0] == '-'
}

func splitFlag(tok string) (name string, hasInline bool) {
	s := strings.TrimLeft(tok, "-")
	name, _, hasInline = strings.Cut(s, "=")
	return name, hasInline
}
