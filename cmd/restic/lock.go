package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/restic/restic"
	"github.com/restic/restic/debug"
	"github.com/restic/restic/repository"
)

var globalLocks []*restic.Lock

func lockRepo(repo *repository.Repository) (*restic.Lock, error) {
	return lockRepository(repo, false)
}

func lockRepoExclusive(repo *repository.Repository) (*restic.Lock, error) {
	return lockRepository(repo, true)
}

func lockRepository(repo *repository.Repository, exclusive bool) (*restic.Lock, error) {
	lockFn := restic.NewLock
	if exclusive {
		lockFn = restic.NewExclusiveLock
	}

	lock, err := lockFn(repo)
	if err != nil {
		if restic.IsAlreadyLocked(err) {
			tpe := ""
			if exclusive {
				tpe = " exclusive"
			}
			fmt.Fprintf(os.Stderr, "unable to acquire%s lock for operation:\n", tpe)
			fmt.Fprintln(os.Stderr, err)
			fmt.Fprintf(os.Stderr, "\nthe `unlock` command can be used to remove stale locks\n")
			os.Exit(1)
		}

		return nil, err
	}

	globalLocks = append(globalLocks, lock)

	return lock, err
}

func unlockRepo(lock *restic.Lock) error {
	if err := lock.Unlock(); err != nil {
		return err
	}

	for i := 0; i < len(globalLocks); i++ {
		if lock == globalLocks[i] {
			globalLocks = append(globalLocks[:i], globalLocks[i+1:]...)
			return nil
		}
	}

	return nil
}

func unlockAll() error {
	debug.Log("unlockAll", "unlocking %d locks", len(globalLocks))
	for _, lock := range globalLocks {
		if err := lock.Unlock(); err != nil {
			return err
		}
	}

	return nil
}

func init() {
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGINT)

	go CleanupHandler(c)
}

// CleanupHandler handles the SIGINT signal.
func CleanupHandler(c <-chan os.Signal) {
	for s := range c {
		debug.Log("CleanupHandler", "signal %v received, cleaning up", s)
		fmt.Println("\x1b[2KInterrupt received, cleaning up")
		unlockAll()
		fmt.Println("exiting")
		os.Exit(0)
	}
}
