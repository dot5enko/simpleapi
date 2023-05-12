package simpleapi

import "github.com/gin-gonic/gin"

type RequestContext[T any, X any] struct {
	Http *gin.Context
	App  *AppContext[X]
	Data *T
}

func (r RequestContext[T, X]) AbortWithCode(code int, resp any) {
	r.Http.AbortWithStatusJSON(code, resp)
}

func (r RequestContext[T, X]) Ok(resp any) {
	r.AbortWithCode(200, resp)
}

type RequestHandler[T any, X any] func(ctx *RequestContext[T, X])

type HandlersChain[T any, X any] []RequestHandler[T, X]

func (handlers *HandlersChain[T, X]) Use(h ...RequestHandler[T, X]) {
	*handlers = append(*handlers, h...)
}

func (handlers HandlersChain[T, X]) ProcessRequest(ginCtx *gin.Context, app *AppContext[X], requestCtx *T) {

	ourCtx := &RequestContext[T, X]{
		Http: ginCtx,
		App:  app,
		Data: requestCtx,
	}

	for _, it := range handlers {
		if !ginCtx.IsAborted() {
			it(ourCtx)
		} else {
			break
		}
	}
}
