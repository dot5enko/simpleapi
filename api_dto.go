package simpleapi

import (
	"github.com/tidwall/gjson"
)

type ApiDto[CtxType any] interface {
	ToApiDto(permission int, ctx *AppContext[CtxType]) Result[map[string]interface{}]
}

type ApiDtoFillable interface {
	FromDto(fields gjson.Result)
}
