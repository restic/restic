// +build debug

package debug

var (
	hooks map[string]func(interface{})
)

func init() {
	hooks = make(map[string]func(interface{}))
}

func Hook(name string, f func(interface{})) {
	hooks[name] = f
}

func RunHook(name string, context interface{}) {
	f, ok := hooks[name]
	if !ok {
		return
	}

	f(context)
}

func RemoveHook(name string) {
	delete(hooks, name)
}
