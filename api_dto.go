package simpleapi

import (
	"github.com/dot5enko/typed"
	"github.com/tidwall/gjson"
)

type ApiDto[CtxType any] interface {
	ToApiDto(permission int, ctx *AppContext[CtxType]) typed.Result[map[string]interface{}]
}

type ApiDtoFillable[CtxType any] interface {
	FromDto(fields gjson.Result, ctx *AppContext[CtxType]) error
}
