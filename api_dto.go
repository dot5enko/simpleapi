package simpleapi

import (
	"github.com/dot5enko/typed"
	"github.com/tidwall/gjson"
)

// allows to include any additional data with default data
// TODO remove result map, change input map by reference
type ApiDto[CtxType any] interface {
	ToApiDto(data map[string]any, permission RequestData, ctx *AppContext[CtxType]) typed.Result[map[string]any]
}

type ApiDtoFillable[CtxType any] interface {
	FromDto(fields gjson.Result, ctx *AppContext[CtxType]) error
}
