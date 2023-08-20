package simpleapi

import (
	"fmt"
	"strconv"

	"github.com/dot5enko/typed"
	"github.com/gin-gonic/gin"
)

func GetObjFromContext[T any](ctx *gin.Context, name string) typed.Result[T] {

	user, isOk := ctx.Get(name)
	if isOk {
		contextObject, ok := user.(T)
		if !ok {
			obj := *new(T)
			return typed.ResultFailed[T](fmt.Errorf("mismatched type of object, have %#v want %#v", user, obj))
		} else {
			return typed.ResultOk(contextObject)
		}
	} else {
		return typed.ResultFailed[T](fmt.Errorf("no user in request context"))
	}

}

func MustGetObjectFromContext[T any](ctx *gin.Context, name string) T {

	if ctx == nil {
		panic("ctx is null, can't get anything of it")
	}

	result := GetObjFromContext[T](ctx, name)
	if !result.IsOk() {
		panic(fmt.Sprintf("error while getting val from context : %s", result.UnwrapError().Error()))
	} else {
		return result.Unwrap()
	}
}

func GetArgUint(ctx *gin.Context, name string) uint64 {
	paramIdStr := ctx.Param(name)
	paramId, err := strconv.ParseUint(paramIdStr, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("unable to get arg uint val : %s", err.Error()))
	} else {
		return paramId
	}
}

func GetUserId(ctx *gin.Context) uint64 {

	userObject := MustGetObjectFromContext[any](ctx, "user")

	features, ok := userObject.(UserFeatures)
	if !ok {
		panic("user object doe's not implement UserFeatures")
	}

	return features.GetId()
}
