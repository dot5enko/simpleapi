package simpleapi

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AppContext[T any] struct {
	Data    *T
	Request *gin.Context

	Db DbWrapper[T]

	isolated bool
}

func (c AppContext[T]) isolateDatabase(isolatedDb *gorm.DB) AppContext[T] {

	result := c

	result.Db.setRaw(isolatedDb)

	return result
}
