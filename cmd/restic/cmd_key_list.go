package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
	"github.com/restic/restic/internal/ui/table"
	"github.com/spf13/cobra"
)

func newKeyListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List keys (passwords)",
		Long: `
The "list" sub-command lists all the keys (passwords) associated with the repository.
Returns the key ID, username, hostname, created time and if it's the current key being
used to access the repository.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
Exit status is 12 if the password is incorrect.
	`,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			term, cancel := setupTermstatus()
			defer cancel()
			return runKeyList(cmd.Context(), globalOptions, args, term)
		},
	}
	return cmd
}

func runKeyList(ctx context.Context, gopts GlobalOptions, args []string, term ui.Terminal) error {
	if len(args) > 0 {
		return fmt.Errorf("the key list command expects no arguments, only options - please see `restic help key list` for usage and flags")
	}

	printer := newTerminalProgressPrinter(gopts.JSON, gopts.verbosity, term)
	ctx, repo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock, printer)
	if err != nil {
		return err
	}
	defer unlock()

	return listKeys(ctx, repo, gopts, printer)
}

func listKeys(ctx context.Context, s *repository.Repository, gopts GlobalOptions, printer progress.Printer) error {
	type keyInfo struct {
		Current  bool   `json:"current"`
		ID       string `json:"id"`
		ShortID  string `json:"-"`
		UserName string `json:"userName"`
		HostName string `json:"hostName"`
		Created  string `json:"created"`
	}

	var m sync.Mutex
	var keys []keyInfo

	err := restic.ParallelList(ctx, s, restic.KeyFile, s.Connections(), func(ctx context.Context, id restic.ID, _ int64) error {
		k, err := repository.LoadKey(ctx, s, id)
		if err != nil {
			printer.E("LoadKey() failed: %v", err)
			return nil
		}

		key := keyInfo{
			Current:  id == s.KeyID(),
			ID:       id.String(),
			ShortID:  id.Str(),
			UserName: k.Username,
			HostName: k.Hostname,
			Created:  k.Created.Local().Format(TimeFormat),
		}

		m.Lock()
		defer m.Unlock()
		keys = append(keys, key)
		return nil
	})

	if err != nil {
		return err
	}

	if gopts.JSON {
		return json.NewEncoder(globalOptions.stdout).Encode(keys)
	}

	tab := table.New()
	tab.AddColumn(" ID", "{{if .Current}}*{{else}} {{end}}{{ .ShortID }}")
	tab.AddColumn("User", "{{ .UserName }}")
	tab.AddColumn("Host", "{{ .HostName }}")
	tab.AddColumn("Created", "{{ .Created }}")

	for _, key := range keys {
		tab.AddRow(key)
	}

	return tab.Write(globalOptions.stdout)
}
