package simpleapi

import "github.com/dot5enko/typed"

func ToDto[T any, CtxType any](it T, appctx *AppContext[CtxType], permission int) typed.Result[map[string]any] {

	m := appctx.Db.ApiData(it)

	rawDto := m.ToDto(it)

	if m.OutExtraMethod {

		dtoPresenter, _ := any(it).(ApiDto[CtxType])

		var result typed.Result[map[string]any]

		var internalError error

		func() {

			defer func() {
				rec := recover()
				if rec != nil {
					internalError = typed.PanickedError{Cause: rec}
				}
			}()

			result = dtoPresenter.ToApiDto(rawDto, permission, appctx)
		}()

		if internalError != nil {
			return typed.ResultFailed[map[string]any](internalError)
		} else {
			return result
		}

	} else {
		return typed.ResultOk(rawDto)
	}
}
