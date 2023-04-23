package debug

import "runtime"

func DumpStacktrace() string {
	buf := make([]byte, 128*1024)

	for {
		l := runtime.Stack(buf, true)
		if l < len(buf) {
			return string(buf[:l])
		}
		buf = make([]byte, len(buf)*2)
	}
}
