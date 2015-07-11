// +build debug_sftp

package sftp

import "log"

func debug(fmt string, args ...interface{}) {
	log.Printf(fmt, args...)
}
