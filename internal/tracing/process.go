package tracing

import (
	"net"
	"os"
	"os/user"
	"strings"
)

// ProcessInfo holds metadata about a process in the ancestry chain.
type ProcessInfo struct {
	PID     int
	PPID    int
	Comm    string // short executable name (e.g. "bash", "sshd")
	CmdLine string // full reconstructed command line (best-effort)
}

// SystemInfo describes the environment in which restic is executing.
type SystemInfo struct {
	User     string // login name
	UserID   string // numeric UID
	FQDN     string
	Ancestry []ProcessInfo // oldest ancestor first, direct parent last
}

// Collect gathers the current user, hostname, and process ancestry chain.
func Collect() SystemInfo {
	info := SystemInfo{}
	if u, err := user.Current(); err == nil {
		info.User = u.Username
		info.UserID = u.Uid
	}
	if h, err := os.Hostname(); err == nil {
		info.FQDN = resolveFQDN(h)
	}
	info.Ancestry = collectAncestry()
	return info
}

// resolveFQDN attempts a DNS reverse lookup to determine the FQDN. Falls back
// to the short hostname if DNS is unavailable or returns no useful result.
func resolveFQDN(hostname string) string {
	if ips, err := net.LookupHost(hostname); err == nil && len(ips) > 0 {
		if names, err := net.LookupAddr(ips[0]); err == nil && len(names) > 0 {
			return strings.TrimSuffix(names[0], ".")
		}
	}
	return hostname
}
