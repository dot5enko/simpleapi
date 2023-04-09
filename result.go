package simpleapi

import (
	"fmt"
	"reflect"
)

type Result[T any] struct {
	has bool
	val *T
	err error
}

func (r Result[T]) String() string {

	obj := new(T)

	return fmt.Sprintf("#result[%s]{has: %v}", reflect.ValueOf(obj).Type().Name(), r.has)
}

func (opt Result[T]) IsOk() bool {
	return opt.has
}

func (opt Result[T]) Unwrap() T {

	if !opt.has {
		panic("unwrapping failed result")
	}

	return *opt.val
}

func (opt Result[T]) UnwrapError() error {
	return opt.err
}

func (opt *Result[T]) SetOk(v T) *Result[T] {
	opt.val = &v
	opt.has = true
	return opt
}

func (opt *Result[T]) SetFail(v error) *Result[T] {
	opt.err = v
	opt.has = false
	return opt
}

func ResultOk[T any](v T) Result[T] {
	res := Result[T]{}
	res.SetOk(v)

	return res
}

func ResultFailed[T any](err error) Result[T] {
	res := Result[T]{}
	res.SetFail(err)
	return res
}
