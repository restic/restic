//go:build !linux

package tracing

func collectAncestry() []ProcessInfo { return nil }
