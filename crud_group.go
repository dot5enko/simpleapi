package simpleapi

import "github.com/gin-gonic/gin"

type CrudGroup[T any] struct {
	Ctx                  AppContext[T]
	Config               CrudGroupConfig[T]
	RequestDataGenerator func(g *gin.Context, ctx *AppContext[T]) RequestData
}

type HasPermissionChecker[T any] func(req *gin.Context, ctx *AppContext[T]) bool

type CrudGroupConfig[T any] struct {
	ObjectIdFieldName string

	WritePermission *HasPermissionChecker[T]
	ReadPermission  *HasPermissionChecker[T]
}

func NewCrudGroup[T any](ctx AppContext[T], config CrudGroupConfig[T]) *CrudGroup[T] {

	result := &CrudGroup[T]{
		Ctx:    ctx,
		Config: config,
	}

	return result

}
