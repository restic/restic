package flags

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode/utf8"
)

type parseState struct {
	arg        string
	args       []string
	retargs    []string
	positional []*Arg
	err        error

	command *Command
	lookup  lookup
}

func (p *parseState) eof() bool {
	return len(p.args) == 0
}

func (p *parseState) pop() string {
	if p.eof() {
		return ""
	}

	p.arg = p.args[0]
	p.args = p.args[1:]

	return p.arg
}

func (p *parseState) peek() string {
	if p.eof() {
		return ""
	}

	return p.args[0]
}

func (p *parseState) checkRequired(parser *Parser) error {
	c := parser.Command

	var required []*Option

	for c != nil {
		c.eachGroup(func(g *Group) {
			for _, option := range g.options {
				if !option.isSet && option.Required {
					required = append(required, option)
				}
			}
		})

		c = c.Active
	}

	if len(required) == 0 {
		if len(p.positional) > 0 && p.command.ArgsRequired {
			var reqnames []string

			for _, arg := range p.positional {
				if arg.isRemaining() {
					break
				}

				reqnames = append(reqnames, "`"+arg.Name+"`")
			}

			if len(reqnames) == 0 {
				return nil
			}

			var msg string

			if len(reqnames) == 1 {
				msg = fmt.Sprintf("the required argument %s was not provided", reqnames[0])
			} else {
				msg = fmt.Sprintf("the required arguments %s and %s were not provided",
					strings.Join(reqnames[:len(reqnames)-1], ", "), reqnames[len(reqnames)-1])
			}

			p.err = newError(ErrRequired, msg)
			return p.err
		}

		return nil
	}

	names := make([]string, 0, len(required))

	for _, k := range required {
		names = append(names, "`"+k.String()+"'")
	}

	sort.Strings(names)

	var msg string

	if len(names) == 1 {
		msg = fmt.Sprintf("the required flag %s was not specified", names[0])
	} else {
		msg = fmt.Sprintf("the required flags %s and %s were not specified",
			strings.Join(names[:len(names)-1], ", "), names[len(names)-1])
	}

	p.err = newError(ErrRequired, msg)
	return p.err
}

func (p *parseState) estimateCommand() error {
	commands := p.command.sortedCommands()
	cmdnames := make([]string, len(commands))

	for i, v := range commands {
		cmdnames[i] = v.Name
	}

	var msg string
	var errtype ErrorType

	if len(p.retargs) != 0 {
		c, l := closestChoice(p.retargs[0], cmdnames)
		msg = fmt.Sprintf("Unknown command `%s'", p.retargs[0])
		errtype = ErrUnknownCommand

		if float32(l)/float32(len(c)) < 0.5 {
			msg = fmt.Sprintf("%s, did you mean `%s'?", msg, c)
		} else if len(cmdnames) == 1 {
			msg = fmt.Sprintf("%s. You should use the %s command",
				msg,
				cmdnames[0])
		} else {
			msg = fmt.Sprintf("%s. Please specify one command of: %s or %s",
				msg,
				strings.Join(cmdnames[:len(cmdnames)-1], ", "),
				cmdnames[len(cmdnames)-1])
		}
	} else {
		errtype = ErrCommandRequired

		if len(cmdnames) == 1 {
			msg = fmt.Sprintf("Please specify the %s command", cmdnames[0])
		} else {
			msg = fmt.Sprintf("Please specify one command of: %s or %s",
				strings.Join(cmdnames[:len(cmdnames)-1], ", "),
				cmdnames[len(cmdnames)-1])
		}
	}

	return newError(errtype, msg)
}

func (p *Parser) parseOption(s *parseState, name string, option *Option, canarg bool, argument *string) (err error) {
	if !option.canArgument() {
		if argument != nil {
			return newErrorf(ErrNoArgumentForBool, "bool flag `%s' cannot have an argument", option)
		}

		err = option.set(nil)
	} else if argument != nil || (canarg && !s.eof()) {
		var arg string

		if argument != nil {
			arg = *argument
		} else {
			arg = s.pop()

			if argumentIsOption(arg) {
				return newErrorf(ErrExpectedArgument, "expected argument for flag `%s', but got option `%s'", option, arg)
			} else if p.Options&PassDoubleDash != 0 && arg == "--" {
				return newErrorf(ErrExpectedArgument, "expected argument for flag `%s', but got double dash `--'", option)
			}
		}

		if option.tag.Get("unquote") != "false" {
			arg, err = unquoteIfPossible(arg)
		}

		if err == nil {
			err = option.set(&arg)
		}
	} else if option.OptionalArgument {
		option.empty()

		for _, v := range option.OptionalValue {
			err = option.set(&v)

			if err != nil {
				break
			}
		}
	} else {
		err = newErrorf(ErrExpectedArgument, "expected argument for flag `%s'", option)
	}

	if err != nil {
		if _, ok := err.(*Error); !ok {
			err = newErrorf(ErrMarshal, "invalid argument for flag `%s' (expected %s): %s",
				option,
				option.value.Type(),
				err.Error())
		}
	}

	return err
}

func (p *Parser) parseLong(s *parseState, name string, argument *string) error {
	if option := s.lookup.longNames[name]; option != nil {
		// Only long options that are required can consume an argument
		// from the argument list
		canarg := !option.OptionalArgument

		return p.parseOption(s, name, option, canarg, argument)
	}

	return newErrorf(ErrUnknownFlag, "unknown flag `%s'", name)
}

func (p *Parser) splitShortConcatArg(s *parseState, optname string) (string, *string) {
	c, n := utf8.DecodeRuneInString(optname)

	if n == len(optname) {
		return optname, nil
	}

	first := string(c)

	if option := s.lookup.shortNames[first]; option != nil && option.canArgument() {
		arg := optname[n:]
		return first, &arg
	}

	return optname, nil
}

func (p *Parser) parseShort(s *parseState, optname string, argument *string) error {
	if argument == nil {
		optname, argument = p.splitShortConcatArg(s, optname)
	}

	for i, c := range optname {
		shortname := string(c)

		if option := s.lookup.shortNames[shortname]; option != nil {
			// Only the last short argument can consume an argument from
			// the arguments list, and only if it's non optional
			canarg := (i+utf8.RuneLen(c) == len(optname)) && !option.OptionalArgument

			if err := p.parseOption(s, shortname, option, canarg, argument); err != nil {
				return err
			}
		} else {
			return newErrorf(ErrUnknownFlag, "unknown flag `%s'", shortname)
		}

		// Only the first option can have a concatted argument, so just
		// clear argument here
		argument = nil
	}

	return nil
}

func (p *parseState) addArgs(args ...string) error {
	for len(p.positional) > 0 && len(args) > 0 {
		arg := p.positional[0]

		if err := convert(args[0], arg.value, arg.tag); err != nil {
			return err
		}

		if !arg.isRemaining() {
			p.positional = p.positional[1:]
		}

		args = args[1:]
	}

	p.retargs = append(p.retargs, args...)
	return nil
}

func (p *Parser) parseNonOption(s *parseState) error {
	if len(s.positional) > 0 {
		return s.addArgs(s.arg)
	}

	if cmd := s.lookup.commands[s.arg]; cmd != nil {
		s.command.Active = cmd
		cmd.fillParseState(s)
	} else if (p.Options & PassAfterNonOption) != None {
		// If PassAfterNonOption is set then all remaining arguments
		// are considered positional
		if err := s.addArgs(s.arg); err != nil {
			return err
		}

		if err := s.addArgs(s.args...); err != nil {
			return err
		}

		s.args = []string{}
	} else {
		return s.addArgs(s.arg)
	}

	return nil
}

func (p *Parser) showBuiltinHelp() error {
	var b bytes.Buffer

	p.WriteHelp(&b)
	return newError(ErrHelp, b.String())
}

func (p *Parser) printError(err error) error {
	if err != nil && (p.Options&PrintErrors) != None {
		fmt.Fprintln(os.Stderr, err)
	}

	return err
}

func (p *Parser) clearIsSet() {
	p.eachCommand(func(c *Command) {
		c.eachGroup(func(g *Group) {
			for _, option := range g.options {
				option.isSet = false
			}
		})
	}, true)
}
