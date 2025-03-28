package simpleapi

import (
	typed "github.com/cldfn/utils"
)

// todo add softdeleted item handler
func ToDto[T any](it T, appctx *AppContext, req RequestData) typed.Result[map[string]any] {

	// optimize
	m := appctx.ApiData(it)

	rawDto := m.ToDto(it, req)

	if m.OutExtraMethod {

		dtoPresenter, _ := any(it).(ApiDto)

		var result typed.Result[map[string]any]

		var internalError error

		func() {

			defer typed.RecoverPanic(func(pe *typed.PanicError) {
				internalError = pe
			})

			result = dtoPresenter.ToApiDto(rawDto, req, appctx)
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
