package flags

import (
	"reflect"
	"strings"
	"syscall"
)

// Set the value of an option to the specified value. An error will be returned
// if the specified value could not be converted to the corresponding option
// value type.
func (option *Option) set(value *string) error {
	option.isSet = true

	if option.isFunc() {
		return option.call(value)
	} else if value != nil {
		return convert(*value, option.value, option.tag)
	}

	return convert("", option.value, option.tag)
}

func (option *Option) canCli() bool {
	return option.ShortName != 0 || len(option.LongName) != 0
}

func (option *Option) canArgument() bool {
	if u := option.isUnmarshaler(); u != nil {
		return true
	}

	return !option.isBool()
}

func (option *Option) emptyValue() reflect.Value {
	tp := option.value.Type()

	if tp.Kind() == reflect.Map {
		return reflect.MakeMap(tp)
	}

	return reflect.Zero(tp)
}

func (option *Option) empty() {
	if !option.isFunc() {
		option.value.Set(option.emptyValue())
	}
}

func (option *Option) clearDefault() {
	usedDefault := option.Default
	if envKey := option.EnvDefaultKey; envKey != "" {
		// os.Getenv() makes no distinction between undefined and
		// empty values, so we use syscall.Getenv()
		if value, ok := syscall.Getenv(envKey); ok {
			if option.EnvDefaultDelim != "" {
				usedDefault = strings.Split(value,
					option.EnvDefaultDelim)
			} else {
				usedDefault = []string{value}
			}
		}
	}

	if len(usedDefault) > 0 {
		option.empty()

		for _, d := range usedDefault {
			option.set(&d)
		}
	} else {
		tp := option.value.Type()

		switch tp.Kind() {
		case reflect.Map:
			if option.value.IsNil() {
				option.empty()
			}
		case reflect.Slice:
			if option.value.IsNil() {
				option.empty()
			}
		}
	}
}

func (option *Option) valueIsDefault() bool {
	// Check if the value of the option corresponds to its
	// default value
	emptyval := option.emptyValue()

	checkvalptr := reflect.New(emptyval.Type())
	checkval := reflect.Indirect(checkvalptr)

	checkval.Set(emptyval)

	if len(option.Default) != 0 {
		for _, v := range option.Default {
			convert(v, checkval, option.tag)
		}
	}

	return reflect.DeepEqual(option.value.Interface(), checkval.Interface())
}

func (option *Option) isUnmarshaler() Unmarshaler {
	v := option.value

	for {
		if !v.CanInterface() {
			break
		}

		i := v.Interface()

		if u, ok := i.(Unmarshaler); ok {
			return u
		}

		if !v.CanAddr() {
			break
		}

		v = v.Addr()
	}

	return nil
}

func (option *Option) isBool() bool {
	tp := option.value.Type()

	for {
		switch tp.Kind() {
		case reflect.Bool:
			return true
		case reflect.Slice:
			return (tp.Elem().Kind() == reflect.Bool)
		case reflect.Func:
			return tp.NumIn() == 0
		case reflect.Ptr:
			tp = tp.Elem()
		default:
			return false
		}
	}
}

func (option *Option) isFunc() bool {
	return option.value.Type().Kind() == reflect.Func
}

func (option *Option) call(value *string) error {
	var retval []reflect.Value

	if value == nil {
		retval = option.value.Call(nil)
	} else {
		tp := option.value.Type().In(0)

		val := reflect.New(tp)
		val = reflect.Indirect(val)

		if err := convert(*value, val, option.tag); err != nil {
			return err
		}

		retval = option.value.Call([]reflect.Value{val})
	}

	if len(retval) == 1 && retval[0].Type() == reflect.TypeOf((*error)(nil)).Elem() {
		if retval[0].Interface() == nil {
			return nil
		}

		return retval[0].Interface().(error)
	}

	return nil
}
