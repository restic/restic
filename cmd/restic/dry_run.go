package main

import "os"

func isDryRunByEnv(dryRun bool) bool {
	if _, exists := os.LookupEnv("RESTIC_DRY_RUN"); exists {
		return true
	}
	return dryRun
}
