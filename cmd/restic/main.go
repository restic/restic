package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/options"
	"github.com/restic/restic/internal/restic"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/restic/restic/internal/errors"
)

// cmdRoot is the base command when no other command has been specified.
var cmdRoot = &cobra.Command{
	Use:   "restic",
	Short: "Backup and restore files",
	Long: `
restic is a backup program which allows saving multiple revisions of files and
directories in an encrypted repository stored on different backends.
`,
	SilenceErrors:     true,
	SilenceUsage:      true,
	DisableAutoGenTag: true,

	PersistentPreRunE: func(c *cobra.Command, args []string) error {

		viper.BindPFlags(c.Flags()) // bind viper to cobra flags
		if globalOptions.ConfigFile == "" {
			viper.SetConfigName("restic")       // name of config file (without extension)
			viper.AddConfigPath("/etc/restic/") // paths to look for the config file in
			viper.AddConfigPath("$HOME/.restic")
			viper.AddConfigPath(".")
		} else {
			viper.SetConfigFile(globalOptions.ConfigFile)
		}
		if err := viper.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				// Config file was found but another error was produced
				panic(fmt.Errorf("Fatal error config file: %s \n", err))
			}
		} else {
			Printf("using config file %v\n", viper.ConfigFileUsed())
		}

		// SetAllCobraFlagsByViper loops over all flags and sets them
		// by viper (if not already set)
		setAllCobraFlagsByViper := func(v *viper.Viper) {
			if v == nil {
				return
			}

			c.Flags().VisitAll(func(f *pflag.Flag) {
				if !f.Changed && v.IsSet(f.Name) {
					value := v.GetString(f.Name)
					debug.Log("use option from config file: set'%v' to '%v'\n", f.Name, value)
					c.Flags().Set(f.Name, value)
				}
			})
		}

		if globalOptions.Profile != "" {
			// set flags of subcommand of profile (e.g. "profiles.prof1.backup.one-file-system")
			setAllCobraFlagsByViper(viper.Sub("profiles." + globalOptions.Profile + "." + c.Name()))
			// set flags of profile (e.g. "profiles.prof1.repository")
			setAllCobraFlagsByViper(viper.Sub("profiles." + globalOptions.Profile))
		}

		// set flags of subcommand (e.g. "backup.one-file-system")
		setAllCobraFlagsByViper(viper.Sub(c.Name()))
		// set global flags  (e.g. "repository")
		setAllCobraFlagsByViper(viper.GetViper())

		// set verbosity, default is one
		globalOptions.verbosity = 1
		if globalOptions.Quiet && (globalOptions.Verbose > 1) {
			return errors.Fatal("--quiet and --verbose cannot be specified at the same time")
		}

		switch {
		case globalOptions.Verbose >= 2:
			globalOptions.verbosity = 3
		case globalOptions.Verbose > 0:
			globalOptions.verbosity = 2
		case globalOptions.Quiet:
			globalOptions.verbosity = 0
		}

		// parse extended options
		opts, err := options.Parse(globalOptions.Options)
		if err != nil {
			return err
		}
		globalOptions.extended = opts
		if c.Name() == "version" {
			return nil
		}
		pwd, err := resolvePassword(globalOptions)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Resolving password failed: %v\n", err)
			Exit(1)
		}
		globalOptions.password = pwd

		// run the debug functions for all subcommands (if build tag "debug" is
		// enabled)
		if err := runDebug(); err != nil {
			return err
		}

		return nil
	},
}

var logBuffer = bytes.NewBuffer(nil)

func init() {
	// install custom global logger into a buffer, if an error occurs
	// we can show the logs
	log.SetOutput(logBuffer)
}

func main() {
	debug.Log("main %#v", os.Args)
	debug.Log("restic %s compiled with %v on %v/%v",
		version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	err := cmdRoot.Execute()

	switch {
	case restic.IsAlreadyLocked(errors.Cause(err)):
		fmt.Fprintf(os.Stderr, "%v\nthe `unlock` command can be used to remove stale locks\n", err)
	case err == ErrInvalidSourceData:
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	case errors.IsFatal(errors.Cause(err)):
		fmt.Fprintf(os.Stderr, "%v\n", err)
	case err != nil:
		fmt.Fprintf(os.Stderr, "%+v\n", err)

		if logBuffer.Len() > 0 {
			fmt.Fprintf(os.Stderr, "also, the following messages were logged by a library:\n")
			sc := bufio.NewScanner(logBuffer)
			for sc.Scan() {
				fmt.Fprintln(os.Stderr, sc.Text())
			}
		}
	}

	var exitCode int
	switch err {
	case nil:
		exitCode = 0
	case ErrInvalidSourceData:
		exitCode = 3
	default:
		exitCode = 1
	}
	Exit(exitCode)
}
