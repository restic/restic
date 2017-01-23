// +build go1.7

package profile

import "runtime/trace"

// Trace profile controls if execution tracing will be enabled. It disables any previous profiling settings.
func TraceProfile(p *profile) { p.mode = traceMode }

var startTrace = trace.Start
var stopTrace = trace.Stop
