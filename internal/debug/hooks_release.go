// +build !debug

package debug

func Hook(name string, f func(interface{})) {}

func RunHook(name string, context interface{}) {}

func RemoveHook(name string) {}
