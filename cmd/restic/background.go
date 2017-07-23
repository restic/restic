// +build !linux

package main

// IsProcessBackground should return true if it is running in the background or false if not
func IsProcessBackground() bool {
	//TODO: Check if the process are running in the background in other OS than linux
	return false
}
