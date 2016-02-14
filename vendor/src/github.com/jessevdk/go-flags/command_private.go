package flags

import (
	"reflect"
	"sort"
	"strings"
	"unsafe"
)

type lookup struct {
	shortNames map[string]*Option
	longNames  map[string]*Option

	commands map[string]*Command
}

func newCommand(name string, shortDescription string, longDescription string, data interface{}) *Command {
	return &Command{
		Group: newGroup(shortDescription, longDescription, data),
		Name:  name,
	}
}

func (c *Command) scanSubcommandHandler(parentg *Group) scanHandler {
	f := func(realval reflect.Value, sfield *reflect.StructField) (bool, error) {
		mtag := newMultiTag(string(sfield.Tag))

		if err := mtag.Parse(); err != nil {
			return true, err
		}

		positional := mtag.Get("positional-args")

		if len(positional) != 0 {
			stype := realval.Type()

			for i := 0; i < stype.NumField(); i++ {
				field := stype.Field(i)

				m := newMultiTag((string(field.Tag)))

				if err := m.Parse(); err != nil {
					return true, err
				}

				name := m.Get("positional-arg-name")

				if len(name) == 0 {
					name = field.Name
				}

				arg := &Arg{
					Name:        name,
					Description: m.Get("description"),

					value: realval.Field(i),
					tag:   m,
				}

				c.args = append(c.args, arg)

				if len(mtag.Get("required")) != 0 {
					c.ArgsRequired = true
				}
			}

			return true, nil
		}

		subcommand := mtag.Get("command")

		if len(subcommand) != 0 {
			ptrval := reflect.NewAt(realval.Type(), unsafe.Pointer(realval.UnsafeAddr()))

			shortDescription := mtag.Get("description")
			longDescription := mtag.Get("long-description")
			subcommandsOptional := mtag.Get("subcommands-optional")
			aliases := mtag.GetMany("alias")

			subc, err := c.AddCommand(subcommand, shortDescription, longDescription, ptrval.Interface())

			if err != nil {
				return true, err
			}

			if len(subcommandsOptional) > 0 {
				subc.SubcommandsOptional = true
			}

			if len(aliases) > 0 {
				subc.Aliases = aliases
			}

			return true, nil
		}

		return parentg.scanSubGroupHandler(realval, sfield)
	}

	return f
}

func (c *Command) scan() error {
	return c.scanType(c.scanSubcommandHandler(c.Group))
}

func (c *Command) eachCommand(f func(*Command), recurse bool) {
	f(c)

	for _, cc := range c.commands {
		if recurse {
			cc.eachCommand(f, true)
		} else {
			f(cc)
		}
	}
}

func (c *Command) eachActiveGroup(f func(cc *Command, g *Group)) {
	c.eachGroup(func(g *Group) {
		f(c, g)
	})

	if c.Active != nil {
		c.Active.eachActiveGroup(f)
	}
}

func (c *Command) addHelpGroups(showHelp func() error) {
	if !c.hasBuiltinHelpGroup {
		c.addHelpGroup(showHelp)
		c.hasBuiltinHelpGroup = true
	}

	for _, cc := range c.commands {
		cc.addHelpGroups(showHelp)
	}
}

func (c *Command) makeLookup() lookup {
	ret := lookup{
		shortNames: make(map[string]*Option),
		longNames:  make(map[string]*Option),
		commands:   make(map[string]*Command),
	}

	parent := c.parent

	for parent != nil {
		if cmd, ok := parent.(*Command); ok {
			cmd.fillLookup(&ret, true)
		}

		if grp, ok := parent.(*Group); ok {
			parent = grp
		} else {
			parent = nil
		}
	}

	c.fillLookup(&ret, false)
	return ret
}

func (c *Command) fillLookup(ret *lookup, onlyOptions bool) {
	c.eachGroup(func(g *Group) {
		for _, option := range g.options {
			if option.ShortName != 0 {
				ret.shortNames[string(option.ShortName)] = option
			}

			if len(option.LongName) > 0 {
				ret.longNames[option.LongNameWithNamespace()] = option
			}
		}
	})

	if onlyOptions {
		return
	}

	for _, subcommand := range c.commands {
		ret.commands[subcommand.Name] = subcommand

		for _, a := range subcommand.Aliases {
			ret.commands[a] = subcommand
		}
	}
}

func (c *Command) groupByName(name string) *Group {
	if grp := c.Group.groupByName(name); grp != nil {
		return grp
	}

	for _, subc := range c.commands {
		prefix := subc.Name + "."

		if strings.HasPrefix(name, prefix) {
			if grp := subc.groupByName(name[len(prefix):]); grp != nil {
				return grp
			}
		} else if name == subc.Name {
			return subc.Group
		}
	}

	return nil
}

type commandList []*Command

func (c commandList) Less(i, j int) bool {
	return c[i].Name < c[j].Name
}

func (c commandList) Len() int {
	return len(c)
}

func (c commandList) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

func (c *Command) sortedCommands() []*Command {
	ret := make(commandList, len(c.commands))
	copy(ret, c.commands)

	sort.Sort(ret)
	return []*Command(ret)
}

func (c *Command) match(name string) bool {
	if c.Name == name {
		return true
	}

	for _, v := range c.Aliases {
		if v == name {
			return true
		}
	}

	return false
}

func (c *Command) hasCliOptions() bool {
	ret := false

	c.eachGroup(func(g *Group) {
		if g.isBuiltinHelp {
			return
		}

		for _, opt := range g.options {
			if opt.canCli() {
				ret = true
			}
		}
	})

	return ret
}

func (c *Command) fillParseState(s *parseState) {
	s.positional = make([]*Arg, len(c.args))
	copy(s.positional, c.args)

	s.lookup = c.makeLookup()
	s.command = c
}
