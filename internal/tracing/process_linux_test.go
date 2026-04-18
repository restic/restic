//go:build linux

package tracing

import (
	"os"
	"testing"
)

func TestCollectAncestry(t *testing.T) {
	chain := collectAncestry()
	if len(chain) == 0 {
		t.Error("expected at least one ancestor process on Linux")
	}
	// Oldest ancestor is first; the last entry is the direct parent.
	for i, p := range chain {
		if p.PID <= 0 {
			t.Errorf("chain[%d]: expected positive PID, got %d", i, p.PID)
		}
	}
}

func TestCollectAncestryOrderedOldestFirst(t *testing.T) {
	chain := collectAncestry()
	if len(chain) < 2 {
		t.Skip("not enough ancestors to verify order")
	}
	// In a proper ancestry chain, each entry's PPID should match the previous entry's PID.
	for i := 1; i < len(chain); i++ {
		if chain[i].PPID != chain[i-1].PID {
			t.Errorf("chain[%d].PPID=%d does not match chain[%d].PID=%d",
				i, chain[i].PPID, i-1, chain[i-1].PID)
		}
	}
}

func TestReadProcInfoCurrentProcess(t *testing.T) {
	pid := os.Getpid()
	info, err := readProcInfo(pid)
	if err != nil {
		t.Fatalf("readProcInfo(%d): %v", pid, err)
	}
	if info.PID != pid {
		t.Errorf("expected PID %d, got %d", pid, info.PID)
	}
	if info.Comm == "" {
		t.Error("expected non-empty Comm for current process")
	}
	if info.CmdLine == "" {
		t.Error("expected non-empty CmdLine for current process")
	}
}

func TestReadProcInfoParentProcess(t *testing.T) {
	ppid := os.Getppid()
	info, err := readProcInfo(ppid)
	if err != nil {
		t.Fatalf("readProcInfo(%d): %v", ppid, err)
	}
	if info.PID != ppid {
		t.Errorf("expected PID %d, got %d", ppid, info.PID)
	}
}

func TestReadProcInfoInvalidPID(t *testing.T) {
	_, err := readProcInfo(-1)
	if err == nil {
		t.Error("expected error for PID -1")
	}
}

func TestReadProcInfoNonExistentPID(t *testing.T) {
	// PID 0 is the idle/swapper process; /proc/0 does not exist.
	_, err := readProcInfo(0)
	if err == nil {
		t.Error("expected error for PID 0")
	}
}
