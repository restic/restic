//go:build linux

package tracing

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func collectAncestry() []ProcessInfo {
	var chain []ProcessInfo
	pid := os.Getppid()
	for pid > 1 && len(chain) < 32 {
		info, err := readProcInfo(pid)
		if err != nil {
			break
		}
		chain = append(chain, info)
		pid = info.PPID
	}
	// Reverse so the oldest ancestor comes first.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

func readProcInfo(pid int) (ProcessInfo, error) {
	info := ProcessInfo{PID: pid}

	commBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return info, err
	}
	info.Comm = strings.TrimSpace(string(commBytes))

	statusBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return info, err
	}
	for _, line := range strings.Split(string(statusBytes), "\n") {
		if strings.HasPrefix(line, "PPid:") {
			if fields := strings.Fields(line); len(fields) >= 2 {
				info.PPID, _ = strconv.Atoi(fields[1])
			}
			break
		}
	}

	// cmdline uses NUL bytes as separators – best-effort, ignore errors.
	if cmdlineBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid)); err == nil && len(cmdlineBytes) > 0 {
		parts := strings.Split(strings.TrimRight(string(cmdlineBytes), "\x00"), "\x00")
		info.CmdLine = strings.Join(parts, " ")
	}

	return info, nil
}
