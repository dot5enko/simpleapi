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

	Debug bool

	_logger         *log.Logger
	_rawDebugLogger *arrayLogger
}

func (r *RequestData) init_debug_logger() {
	if r._logger == nil {
		r._logger, r._rawDebugLogger = new_debug_logger()
	}
}

func (r *RequestData) DebugLogs() []string {
	return r.getDebugLogs()
}

func (r *RequestData) InitLogger() {
	r.init_debug_logger()
}

func (r *RequestData) getDebugLogs() []string {
	if r.Debug {
		return r._rawDebugLogger.lines
	} else {
		return []string{}
	}
}

func (r *RequestData) log(cb func(logger *log.Logger)) {

	if r.Debug {
		cb(r._logger)
	}
}

func (r *RequestData) log_format(format string, args ...any) {
	if r.Debug {
		if len(args) == 0 {
			r._logger.Print(format)
		} else {
			r._logger.Printf(format, args...)
		}
	}
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

	updatedFields := 0

	for _, _fieldName := range m.Fillable {

		if br {
			break
		}

		func() {

			defer func() {
				r := recover()
				if r != nil {
					log.Printf("error processing a field: %s: %v", _fieldName, r)

					req.log(func(loggger *log.Logger) {
						loggger.Printf(" [%s]  error processing: %v", _fieldName, r)
					})
				}
			}()

			fieldInfo := m.Fields[_fieldName]

			// todo optimize
			// make groups inheritance, etc
			if fieldInfo.WriteRole > 0 && fieldInfo.WriteRole != uint64(req.RoleGroup) {

				req.log(func(logger *log.Logger) {
					logger.Printf(" [%s] skipped filling because user nor admin not it has needed group to write this field", _fieldName)
				})

				return
			}

			dtoFieldToUse := *fieldInfo.FillName

			jsonFieldValue := dto.Get(dtoFieldToUse)

			if !jsonFieldValue.Exists() {
				// skip non passed fields to update
				return
			}

			field := reflected.FieldByName(_fieldName)

			dtoData, fieldProcessingErr := ProcessFieldType(fieldInfo, jsonFieldValue, req)
			if fieldProcessingErr != nil {
				log.Printf("error processing a field: %s: %s", _fieldName, fieldProcessingErr.Error())

				if req.Debug {
					req.log(func(logger *log.Logger) {
						logger.Printf(" [%s] error processing a field: %s", _fieldName, fieldProcessingErr.Error())
					})
				}

			} else {
				if dtoData != nil {
					field.Set(reflect.ValueOf(dtoData))
					updatedFields += 1

					req.log(func(logger *log.Logger) {
						logger.Printf(" [%s] set `%v`", _fieldName, dtoData)
					})
				}
			}
		}()
	}

	if updatedFields == 0 {
		req.log(func(logger *log.Logger) {
			logger.Printf("no fields updated during fill , user is admin = %v", req.IsAdmin)
		})
	}

	if m.FillExtraMethod {

		req.log(func(logger *log.Logger) {
			logger.Printf(" has fill extra method, processing")
		})

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

// a little bit of abstractions
type TransactionProcessor[T any] func(c AppContext[T]) error

func (c AppContext[T]) DbTransaction(processor TransactionProcessor[T]) error {

	return c.Db.Raw().Transaction(func(tx *gorm.DB) error {

		isolatedCtx := c.isolateDatabase(tx)
		isolatedCtx.isolated = true

		return processor(isolatedCtx)
	})
}
