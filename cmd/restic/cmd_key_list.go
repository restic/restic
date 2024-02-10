package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/table"
	"github.com/spf13/cobra"
)

var cmdKeyList = &cobra.Command{
	Use:   "list",
	Short: "List keys (passwords)",
	Long: `
The "list" sub-command lists all the keys (passwords) associated with the repository.
Returns the key ID, username, hostname, created time and if it's the current key being
used to access the repository.

EXIT STATUS
===========

Exit status is 0 if the command is successful, and non-zero if there was any error.
	`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runKeyList(cmd.Context(), globalOptions, args)
	},
}

func init() {
	cmdKey.AddCommand(cmdKeyList)
}

func runKeyList(ctx context.Context, gopts GlobalOptions, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("the key list command expects no arguments, only options - please see `restic help key list` for usage and flags")
	}

	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		var lock *restic.Lock
		lock, ctx, err = lockRepo(ctx, repo, gopts.RetryLock, gopts.JSON)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	return listKeys(ctx, repo, gopts)
}

func listKeys(ctx context.Context, s *repository.Repository, gopts GlobalOptions) error {
	type keyInfo struct {
		Current  bool   `json:"current"`
		ID       string `json:"id"`
		UserName string `json:"userName"`
		HostName string `json:"hostName"`
		Created  string `json:"created"`
	}

	var m sync.Mutex
	var keys []keyInfo

	err := restic.ParallelList(ctx, s, restic.KeyFile, s.Connections(), func(ctx context.Context, id restic.ID, _ int64) error {
		k, err := repository.LoadKey(ctx, s, id)
		if err != nil {
			Warnf("LoadKey() failed: %v\n", err)
			return nil
		}

		key := keyInfo{
			Current:  id == s.KeyID(),
			ID:       id.Str(),
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
	tab.AddColumn(" ID", "{{if .Current}}*{{else}} {{end}}{{ .ID }}")
	tab.AddColumn("User", "{{ .UserName }}")
	tab.AddColumn("Host", "{{ .HostName }}")
	tab.AddColumn("Created", "{{ .Created }}")

	for _, key := range keys {
		tab.AddRow(key)
	}

	return tab.Write(globalOptions.stdout)
}
