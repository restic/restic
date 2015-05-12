// Copyright 2012 Jesse van den Kieboom. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package flags

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"reflect"
	"runtime"
	"strings"
	"unicode/utf8"
)

type alignmentInfo struct {
	maxLongLen      int
	hasShort        bool
	hasValueName    bool
	terminalColumns int
	indent          bool
}

const (
	paddingBeforeOption                 = 2
	distanceBetweenOptionAndDescription = 2
)

func (a *alignmentInfo) descriptionStart() int {
	ret := a.maxLongLen + distanceBetweenOptionAndDescription

	if a.hasShort {
		ret += 2
	}

	if a.maxLongLen > 0 {
		ret += 4
	}

	if a.hasValueName {
		ret += 3
	}

	return ret
}

func (a *alignmentInfo) updateLen(name string, indent bool) {
	l := utf8.RuneCountInString(name)

	if indent {
		l = l + 4
	}

	if l > a.maxLongLen {
		a.maxLongLen = l
	}
}

func (p *Parser) getAlignmentInfo() alignmentInfo {
	ret := alignmentInfo{
		maxLongLen:      0,
		hasShort:        false,
		hasValueName:    false,
		terminalColumns: getTerminalColumns(),
	}

	if ret.terminalColumns <= 0 {
		ret.terminalColumns = 80
	}

	var prevcmd *Command

	p.eachActiveGroup(func(c *Command, grp *Group) {
		if c != prevcmd {
			for _, arg := range c.args {
				ret.updateLen(arg.Name, c != p.Command)
			}
		}

		for _, info := range grp.options {
			if !info.canCli() {
				continue
			}

			if info.ShortName != 0 {
				ret.hasShort = true
			}

			if len(info.ValueName) > 0 {
				ret.hasValueName = true
			}

			ret.updateLen(info.LongNameWithNamespace()+info.ValueName, c != p.Command)
		}
	})

	return ret
}

func (p *Parser) writeHelpOption(writer *bufio.Writer, option *Option, info alignmentInfo) {
	line := &bytes.Buffer{}

	prefix := paddingBeforeOption

	if info.indent {
		prefix += 4
	}

	line.WriteString(strings.Repeat(" ", prefix))

	if option.ShortName != 0 {
		line.WriteRune(defaultShortOptDelimiter)
		line.WriteRune(option.ShortName)
	} else if info.hasShort {
		line.WriteString("  ")
	}

	descstart := info.descriptionStart() + paddingBeforeOption

	if len(option.LongName) > 0 {
		if option.ShortName != 0 {
			line.WriteString(", ")
		} else if info.hasShort {
			line.WriteString("  ")
		}

		line.WriteString(defaultLongOptDelimiter)
		line.WriteString(option.LongNameWithNamespace())
	}

	if option.canArgument() {
		line.WriteRune(defaultNameArgDelimiter)

		if len(option.ValueName) > 0 {
			line.WriteString(option.ValueName)
		}
	}

	written := line.Len()
	line.WriteTo(writer)

	if option.Description != "" {
		dw := descstart - written
		writer.WriteString(strings.Repeat(" ", dw))

		def := ""
		defs := option.Default

		if len(option.DefaultMask) != 0 {
			if option.DefaultMask != "-" {
				def = option.DefaultMask
			}
		} else if len(defs) == 0 && option.canArgument() {
			var showdef bool

			switch option.field.Type.Kind() {
			case reflect.Func, reflect.Ptr:
				showdef = !option.value.IsNil()
			case reflect.Slice, reflect.String, reflect.Array:
				showdef = option.value.Len() > 0
			case reflect.Map:
				showdef = !option.value.IsNil() && option.value.Len() > 0
			default:
				zeroval := reflect.Zero(option.field.Type)
				showdef = !reflect.DeepEqual(zeroval.Interface(), option.value.Interface())
			}

			if showdef {
				def, _ = convertToString(option.value, option.tag)
			}
		} else if len(defs) != 0 {
			l := len(defs) - 1

			for i := 0; i < l; i++ {
				def += quoteIfNeeded(defs[i]) + ", "
			}

			def += quoteIfNeeded(defs[l])
		}

		var envDef string
		if option.EnvDefaultKey != "" {
			var envPrintable string
			if runtime.GOOS == "windows" {
				envPrintable = "%" + option.EnvDefaultKey + "%"
			} else {
				envPrintable = "$" + option.EnvDefaultKey
			}
			envDef = fmt.Sprintf(" [%s]", envPrintable)
		}

		var desc string

		if def != "" {
			desc = fmt.Sprintf("%s (%v)%s", option.Description, def, envDef)
		} else {
			desc = option.Description + envDef
		}

		writer.WriteString(wrapText(desc,
			info.terminalColumns-descstart,
			strings.Repeat(" ", descstart)))
	}

	writer.WriteString("\n")
}

func maxCommandLength(s []*Command) int {
	if len(s) == 0 {
		return 0
	}

	ret := len(s[0].Name)

	for _, v := range s[1:] {
		l := len(v.Name)

		if l > ret {
			ret = l
		}
	}

	return ret
}

// WriteHelp writes a help message containing all the possible options and
// their descriptions to the provided writer. Note that the HelpFlag parser
// option provides a convenient way to add a -h/--help option group to the
// command line parser which will automatically show the help messages using
// this method.
func (p *Parser) WriteHelp(writer io.Writer) {
	if writer == nil {
		return
	}

	wr := bufio.NewWriter(writer)
	aligninfo := p.getAlignmentInfo()

	cmd := p.Command

	for cmd.Active != nil {
		cmd = cmd.Active
	}

	if p.Name != "" {
		wr.WriteString("Usage:\n")
		wr.WriteString(" ")

		allcmd := p.Command

		for allcmd != nil {
			var usage string

			if allcmd == p.Command {
				if len(p.Usage) != 0 {
					usage = p.Usage
				} else if p.Options&HelpFlag != 0 {
					usage = "[OPTIONS]"
				}
			} else if us, ok := allcmd.data.(Usage); ok {
				usage = us.Usage()
			} else if allcmd.hasCliOptions() {
				usage = fmt.Sprintf("[%s-OPTIONS]", allcmd.Name)
			}

			if len(usage) != 0 {
				fmt.Fprintf(wr, " %s %s", allcmd.Name, usage)
			} else {
				fmt.Fprintf(wr, " %s", allcmd.Name)
			}

			if len(allcmd.args) > 0 {
				fmt.Fprintf(wr, " ")
			}

			for i, arg := range allcmd.args {
				if i != 0 {
					fmt.Fprintf(wr, " ")
				}

				name := arg.Name

				if arg.isRemaining() {
					name = name + "..."
				}

				if !allcmd.ArgsRequired {
					fmt.Fprintf(wr, "[%s]", name)
				} else {
					fmt.Fprintf(wr, "%s", name)
				}
			}

			if allcmd.Active == nil && len(allcmd.commands) > 0 {
				var co, cc string

				if allcmd.SubcommandsOptional {
					co, cc = "[", "]"
				} else {
					co, cc = "<", ">"
				}

				if len(allcmd.commands) > 3 {
					fmt.Fprintf(wr, " %scommand%s", co, cc)
				} else {
					subcommands := allcmd.sortedCommands()
					names := make([]string, len(subcommands))

					for i, subc := range subcommands {
						names[i] = subc.Name
					}

					fmt.Fprintf(wr, " %s%s%s", co, strings.Join(names, " | "), cc)
				}
			}

			allcmd = allcmd.Active
		}

		fmt.Fprintln(wr)

		if len(cmd.LongDescription) != 0 {
			fmt.Fprintln(wr)

			t := wrapText(cmd.LongDescription,
				aligninfo.terminalColumns,
				"")

			fmt.Fprintln(wr, t)
		}
	}

	c := p.Command

	for c != nil {
		printcmd := c != p.Command

		c.eachGroup(func(grp *Group) {
			first := true

			// Skip built-in help group for all commands except the top-level
			// parser
			if grp.isBuiltinHelp && c != p.Command {
				return
			}

			for _, info := range grp.options {
				if !info.canCli() {
					continue
				}

				if printcmd {
					fmt.Fprintf(wr, "\n[%s command options]\n", c.Name)
					aligninfo.indent = true
					printcmd = false
				}

				if first && cmd.Group != grp {
					fmt.Fprintln(wr)

					if aligninfo.indent {
						wr.WriteString("    ")
					}

					fmt.Fprintf(wr, "%s:\n", grp.ShortDescription)
					first = false
				}

				p.writeHelpOption(wr, info, aligninfo)
			}
		})

		if len(c.args) > 0 {
			if c == p.Command {
				fmt.Fprintf(wr, "\nArguments:\n")
			} else {
				fmt.Fprintf(wr, "\n[%s command arguments]\n", c.Name)
			}

			maxlen := aligninfo.descriptionStart()

			for _, arg := range c.args {
				prefix := strings.Repeat(" ", paddingBeforeOption)
				fmt.Fprintf(wr, "%s%s", prefix, arg.Name)

				if len(arg.Description) > 0 {
					align := strings.Repeat(" ", maxlen-len(arg.Name)-1)
					fmt.Fprintf(wr, ":%s%s", align, arg.Description)
				}

				fmt.Fprintln(wr)
			}
		}

		c = c.Active
	}

	scommands := cmd.sortedCommands()

	if len(scommands) > 0 {
		maxnamelen := maxCommandLength(scommands)

		fmt.Fprintln(wr)
		fmt.Fprintln(wr, "Available commands:")

		for _, c := range scommands {
			fmt.Fprintf(wr, "  %s", c.Name)

			if len(c.ShortDescription) > 0 {
				pad := strings.Repeat(" ", maxnamelen-len(c.Name))
				fmt.Fprintf(wr, "%s  %s", pad, c.ShortDescription)

				if len(c.Aliases) > 0 {
					fmt.Fprintf(wr, " (aliases: %s)", strings.Join(c.Aliases, ", "))
				}

			}

			fmt.Fprintln(wr)
		}
	}

	wr.Flush()
}
