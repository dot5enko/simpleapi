package simpleapi

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AppContext[T any] struct {
	Data    *T
	Request *gin.Context

	Db DbWrapper[T]

	objects map[string]FieldsMapping

	isolated bool
}

func (c DbWrapper[T]) Migrate(object any) {

	m := c.db.Migrator()

	m.AutoMigrate(object)

	objTypeName := GetObjectType(object)
	c.app.objects[objTypeName] = GetFieldTags(object)
}

func (c DbWrapper[T]) ApiData(object any) FieldsMapping {
	objTypeName := GetObjectType(object)
	rules, ok := c.app.objects[objTypeName]

	if !ok {
		panic(fmt.Sprintf("trying to get api info for type (%s) that is not a valid registered object within project. try calling .Migrate() first", objTypeName))
	} else {
		return rules
	}
}

func (c AppContext[T]) isolateDatabase(isolatedDb *gorm.DB) AppContext[T] {

	result := c

	result.Db.setRaw(isolatedDb)

	return result
}
