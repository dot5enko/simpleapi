package simpleapi

import "github.com/dot5enko/typed"

func ToDto[T any, CtxType any](it T, appctx *AppContext[CtxType], permission int) typed.Result[map[string]any] {

	m := appctx.Db.ApiData(it)

	rawDto := m.ToDto(it)

	if m.OutExtraMethod {

		dtoPresenter, _ := any(it).(ApiDto[CtxType])
		return dtoPresenter.ToApiDto(rawDto, permission, appctx)
	} else {
		return typed.ResultOk(rawDto)
	}
}
