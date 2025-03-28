package simpleapi

import (
	typed "github.com/cldfn/utils"
	"github.com/tidwall/gjson"
)

// allows to include any additional data with default data
// TODO remove result map, change input map by reference
type ApiDto interface {
	ToApiDto(data map[string]any, permission RequestData, ctx *AppContext) typed.Result[map[string]any]
}

type ApiDtoFillable interface {
	FromDto(fields gjson.Result, ctx *AppContext) error
}
