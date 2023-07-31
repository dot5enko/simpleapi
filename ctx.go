package simpleapi

import (
	"fmt"
	"log"
	"reflect"
	"time"

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

var (
	ErrNumberOverflow = fmt.Errorf("field value overflows type")
)

func (c AppContext[T]) RegisteredTypes() map[string]FieldsMapping {
	return c.objects
}

func (c AppContext[T]) FillEntityFromDto(obj any, dto gjson.Result, options *FillFromDtoOptions) (err error) {

	m := c.Db.ApiData(obj)

	reflected := reflect.Indirect(reflect.ValueOf(obj))

	if !reflected.CanSet() {
		return fmt.Errorf("object is not addressable, can't fill from dto")
	}

	br := false

	for _, _fieldName := range m.Fillable {

		if br {
			break
		}

		func(fieldName string) {
			defer func() {
				r := recover()

				if r != nil {
					err = fmt.Errorf("unable to fill `%s` from dto: %s. fields available", fieldName, r)
					br = true
				}
			}()

			fieldInfo := m.Fields[fieldName]

			field := reflected.FieldByName(fieldName)

			dtoFieldToUse := *fieldInfo.FillName

			fieldType := field.Type()
			fieldTypeKind := fieldType.Kind()

			// if field.IsZero() {
			// 	err = fmt.Errorf("unable to find a `%s` field on type : %s. field type and kind : %s %s type declared : %s",
			// 		fieldName, reflected.Type().Name(), fieldType.Name(), fieldTypeKind.String(), m.TypeName,
			// 	)
			// 	br = true
			// 	return
			// }

			var dtoData any

			jsonFieldValue := dto.Get(dtoFieldToUse)

			if !jsonFieldValue.Exists() {
				// skip non passed fields to update
				return
			}

			switch fieldTypeKind {
			case reflect.Struct:

				if fieldInfo.Typ == "time/Time" {

					unixts := dto.Get(dtoFieldToUse).Int()
					tread := time.Unix(unixts, 0)

					dtoData = tread

				} else {
					log.Panicf("unsupported field (%s) type to set from json: %s", fieldName, fieldInfo.Typ)
				}
			case reflect.Int:
				intVal := jsonFieldValue.Int()
				dtoData = int(intVal)

			case reflect.Uint8:
				uintval := jsonFieldValue.Uint()

				if uintval > 255 {
					err = ErrNumberOverflow
					br = true
					return
				} else {
					dtoData = uint8(uintval)
				}

			default:

				if fieldName == "AppType" {
					log.Printf("app type field kind : %d", fieldTypeKind)
				}

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
			}

			if dtoData != nil {
				field.Set(reflect.ValueOf(dtoData))
			}

		}(_fieldName)
	}

	if m.FillExtraMethod {
		modelFillable, _ := any(obj).(ApiDtoFillable[T])

		// todo move in transaction
		return modelFillable.FromDto(dto, &c)
	}

	return
}

type MigrateInitializer func(object any) FieldsMapping

func (c DbWrapper[T]) MigrateWithOnUpdate(object any, initalizer MigrateInitializer) {

	m := c.db.Migrator()

	m.AutoMigrate(object)

	objTypeName := GetObjectType(object)
	el := initalizer(object)
	el.TypeName = objTypeName
	c.app.objects[objTypeName] = el
}

func (c DbWrapper[T]) MigrateAll(objects ...any) {
	for _, it := range objects {
		c.Migrate(it)
	}
}

func (c DbWrapper[T]) Migrate(object any) {

	m := c.db.Migrator()

	m.AutoMigrate(object)

	objTypeName := GetObjectType(object)
	el := GetFieldTags[T, any](object)
	el.TypeName = objTypeName
	c.app.objects[objTypeName] = el
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
