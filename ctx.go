package simpleapi

import (
	"fmt"
	"log"
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

type RequestData struct {
	IsAdmin          bool
	RoleGroup        uint8
	AuthorizedUserId any // todo use generic type
}

type AppContext[T any] struct {
	Data    *T
	Request *gin.Context

	Db DbWrapper[T]

	AppRequest RequestData

	objects map[string]FieldsMapping

	isolated bool
}

func (actx *AppContext[T]) SetObjectsMapping(omap map[string]FieldsMapping) {
	actx.objects = omap
}

func (actx AppContext[T]) ApiData(object any) FieldsMapping {
	objTypeName := GetObjectType(object)
	rules, ok := actx.objects[objTypeName]

	if !ok {
		panic(fmt.Sprintf("trying to get api info for type (%s) that is not a valid registered object within project. try calling .Migrate() first", objTypeName))
	} else {
		return rules
	}
}

var (
	ErrNumberOverflow = fmt.Errorf("field value overflows type")
)

func (c AppContext[T]) RegisteredTypes() map[string]FieldsMapping {
	return c.objects
}

func (c AppContext[T]) FillEntityFromDto(modelTypeData FieldsMapping, obj any, dto gjson.Result, options *FillFromDtoOptions, req RequestData) (err error) {

	m := modelTypeData

	reflected := reflect.Indirect(reflect.ValueOf(obj))

	if !reflected.CanSet() {
		return fmt.Errorf("object is not addressable, can't fill from dto")
	}

	br := false

	for _, _fieldName := range m.Fillable {

		if br {
			break
		}

		func() {

			defer func() {
				r := recover()
				if r != nil {
					log.Printf("error processing a field: %s: %v", _fieldName, r)
				}
			}()

			fieldInfo := m.Fields[_fieldName]

			// todo optimize
			// make groups inheritance, etc
			if fieldInfo.WriteRole > 0 && fieldInfo.WriteRole != uint64(req.RoleGroup) {
				return
			}

			dtoFieldToUse := *fieldInfo.FillName

			jsonFieldValue := dto.Get(dtoFieldToUse)

			if !jsonFieldValue.Exists() {
				// skip non passed fields to update
				return
			}

			field := reflected.FieldByName(_fieldName)

			dtoData, fieldProcessingErr := ProcessFieldType(fieldInfo, jsonFieldValue)
			if fieldProcessingErr != nil {
				log.Printf("error processing a field: %s: %s", _fieldName, fieldProcessingErr.Error())
			} else {
				if dtoData != nil {
					field.Set(reflect.ValueOf(dtoData))
				}
			}
		}()

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

func (c AppContext[T]) isolateDatabase(isolatedDb *gorm.DB) AppContext[T] {

	result := c

	result.Db.setRaw(isolatedDb)

	return result
}
