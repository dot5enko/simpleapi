package simpleapi

import "github.com/gin-gonic/gin"

type CrudGroup[T any] struct {
	Ctx    AppContext
	Config CrudGroupConfig[T]
}

type HasPermissionChecker[T any] func(req *gin.Context, ctx *AppContext) bool

type CrudGroupConfig[T any] struct {
	ObjectIdFieldName string

	WritePermission      *HasPermissionChecker[T]
	ReadPermission       *HasPermissionChecker[T]
	RequestDataGenerator func(g *gin.Context, ctx *AppContext) RequestData
}

func NewCrudGroup[T any](ctx AppContext, config CrudGroupConfig[T]) *CrudGroup[T] {

	result := &CrudGroup[T]{
		Ctx:    ctx,
		Config: config,
	}

	return result

}
