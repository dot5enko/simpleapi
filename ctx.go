package simpleapi

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AppContext[T any] struct {
	Db      *gorm.DB
	Data    *T
	Request *gin.Context
}
