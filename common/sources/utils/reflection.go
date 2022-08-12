package utils

import (
	"reflect"
	"github.com/pkg/errors"
)

func CallMethod(i any, methodName string) (any, error) {
	var ptr reflect.Value
	var value reflect.Value
	var finalMethod reflect.Value
	value = reflect.ValueOf(i)
	// if we start with a pointer, we need to get value pointed to
	// if we start with a value, we need to get a pointer to that value
	if value.Type().Kind() == reflect.Ptr {
		ptr = value
		value = ptr.Elem()
	} else {
		ptr = reflect.New(reflect.TypeOf(i))
		temp := ptr.Elem()
		temp.Set(value)
	}
	// check for method on value
	method := value.MethodByName(methodName)
	if method.IsValid() {
		finalMethod = method
	}
	// check for method on pointer
	method = ptr.MethodByName(methodName)
	if method.IsValid() {
		finalMethod = method
	}
	if finalMethod.IsValid() {
		return finalMethod.Call([]reflect.Value{})[0].Interface(), nil
	}
	// method not found of either type
	return Zero[any](), errors.Errorf("method not found: %s", methodName)
}
