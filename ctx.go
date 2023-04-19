package simpleapi

import (
	"fmt"
	"reflect"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

func NewAppContext[T any](ctx *T) *AppContext[T] {

	app := &AppContext[T]{
		Data:    ctx,
		objects: map[string]FieldsMapping{},
	}

	return app
}

type AppContext[T any] struct {
	Data    *T
	Request *gin.Context

	Db DbWrapper[T]

	objects map[string]FieldsMapping

	isolated bool
}

func (c AppContext[T]) FillEntityFromDto(obj any, dto gjson.Result, options *FillFromDtoOptions) (err error) {

	m := c.Db.ApiData(obj)

	reflected := reflect.Indirect(reflect.ValueOf(obj))

	if !reflected.CanSet() {
		return fmt.Errorf("object is not addressable, can't fill from dto")
	}

	for _, fieldName := range m.Fillable {

		func() {
			defer func() {
				r := recover()

				if r != nil {
					err = fmt.Errorf("unable to fill `%s` from dto. reflection error: %s", fieldName, r)
				}
			}()

			fieldInfo := m.Fields[fieldName]

			field := reflected.FieldByName(fieldName)

			dtoFieldToUse := *fieldInfo.FillName

			fieldType := field.Type()

			fieldTypeKind := fieldType.Kind()

			var dtoData any

			if fieldTypeKind >= 2 && fieldTypeKind <= 6 {
				// cast to int
				dtoData = dto.Get(dtoFieldToUse).Int()
			} else {
				if fieldTypeKind >= 7 && fieldTypeKind <= 11 {
					dtoData = dto.Get(dtoFieldToUse).Uint()
				} else {
					dtoData = dto.Get(dtoFieldToUse).Value()
				}
			}

			field.Set(reflect.ValueOf(dtoData))
		}()
	}

	if m.FillExtraMethod {
		modelFillable, _ := any(obj).(ApiDtoFillable[T])

		// todo move in transaction
		return modelFillable.FromDto(dto, &c)
	}

	return
}

func (c DbWrapper[T]) Migrate(object any) {

	m := c.db.Migrator()

	m.AutoMigrate(object)

	objTypeName := GetObjectType(object)
	c.app.objects[objTypeName] = GetFieldTags[T](object)
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
