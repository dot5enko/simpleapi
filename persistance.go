package simpleapi

import (
	"fmt"

	"github.com/dot5enko/typed"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BeforeCreateCbAware[CtxType any] interface {
	BeforeEntityCreate(ctx *AppContext[CtxType]) error
}

type AfterCreateCbAware[CtxType any] interface {
	AfterEntityCreate(ctx *AppContext[CtxType]) error
}

type OnAfterUpdateCbAware[CtxType any] interface {
	AfterUpdate(ctx *AppContext[CtxType]) error
}

type OnBeforeUpdateCbAware[CtxType any] interface {
	BeforeUpdate(ctx *AppContext[CtxType]) error
}

type OnUpdateEventHandler[CtxType any, T any] interface {
	OnUpdate(ctx *AppContext[CtxType], prevState T) error
}

type DbWrapper[CtxType any] struct {
	db    *gorm.DB
	topDb *gorm.DB

	app   *AppContext[CtxType]
	debug bool
}

func WrapGormDb[T any](d *gorm.DB, ctx *AppContext[T]) DbWrapper[T] {
	return DbWrapper[T]{
		db:    d,
		topDb: d,
		app:   ctx,
		debug: false,
	}
}

func (d DbWrapper[CtxType]) Raw() *gorm.DB {
	return d.db
}

func (d *DbWrapper[CtxType]) Debug(v bool) {
	d.debug = v
}
func (d DbWrapper[CtxType]) CleanCopy() DbWrapper[CtxType] {
	return WrapGormDb[CtxType](d.topDb, d.app)
}

func (d *DbWrapper[CtxType]) setRaw(_db *gorm.DB) {
	d.db = _db
}

func _isolatedCreate[CtxType any](obj any, ctx AppContext[CtxType]) (err error) {

	_db := ctx.Db.Raw()

	__obj, ok := obj.(OnBeforeUpdateCbAware[CtxType])
	if ok {
		beforeUpdateCbErr := __obj.BeforeUpdate(&ctx)
		if beforeUpdateCbErr != nil {
			return beforeUpdateCbErr
		}
	}

	err = _db.Create(obj).Error

	if err != nil {
		return err
	}

	// check after event
	_obj, ok := obj.(AfterCreateCbAware[CtxType])
	if ok {
		return _obj.AfterEntityCreate(&ctx)
	}

	return nil
}

func _isolatedSave[CtxType any](obj any, ctx AppContext[CtxType]) (err error) {

	_db := ctx.Db.Raw()

	__obj, ok := obj.(OnBeforeUpdateCbAware[CtxType])
	if ok {
		beforeUpdateCbErr := __obj.BeforeUpdate(&ctx)
		if beforeUpdateCbErr != nil {
			return beforeUpdateCbErr
		}
	}

	// isCreate := false

	// {
	// 	// detect save or update

	// 	tx := _db
	// 	tx.Statement.Dest = obj

	// 	reflectValue := reflect.Indirect(reflect.ValueOf(obj))
	// 	for reflectValue.Kind() == reflect.Ptr || reflectValue.Kind() == reflect.Interface {
	// 		reflectValue = reflect.Indirect(reflectValue)
	// 	}

	// 	switch reflectValue.Kind() {
	// 	case reflect.Struct:
	// 		if err := _db.Statement.Parse(obj); err == nil && _db.Statement.Schema != nil {
	// 			for _, pf := range _db.Statement.Schema.PrimaryFields {
	// 				if _, isZero := pf.ValueOf(_db.Statement.Context, reflectValue); isZero {
	// 					isCreate = true
	// 				}
	// 			}
	// 		}
	// 	}
	// }

	// if isCreate {

	// 	{
	// 		__obj, ok := obj.(BeforeCreateCbAware[CtxType])
	// 		if ok {
	// 			cb := __obj.BeforeEntityCreate(&ctx)
	// 			if cb != nil {
	// 				return cb
	// 			}
	// 		}
	// 	}

	// 	err = _db.Create(obj).Error
	// 	if err != nil {
	// 		return err
	// 	}

	// 	{
	// 		__obj, ok := obj.(AfterCreateCbAware[CtxType])
	// 		if ok {
	// 			cb := __obj.AfterEntityCreate(&ctx)
	// 			if cb != nil {
	// 				return cb
	// 			}
	// 		}
	// 	}

	// } else {
	err = _db.Save(obj).Error
	// }

	if err != nil {
		return err
	}

	// check after event
	// should be executed after transaction commit
	_obj, ok := obj.(OnAfterUpdateCbAware[CtxType])
	if ok {
		return _obj.AfterUpdate(&ctx)
	}

	return nil
}

func (d DbWrapper[CtxType]) Save(obj any) (err error) {

	if d.app.isolated {
		return _isolatedSave(obj, *d.app)
	}

	return d.db.Transaction(func(tx *gorm.DB) error {
		return _isolatedSave(obj, d.app.isolateDatabase(tx))
	})
}

func (d DbWrapper[CtxType]) Create(obj any) (err error) {

	if d.app.isolated {
		return _isolatedCreate(obj, *d.app)
	}

	return d.db.Transaction(func(tx *gorm.DB) error {
		return _isolatedCreate(obj, d.app.isolateDatabase(tx))
	})
}

func (d DbWrapper[CtxType]) Delete(obj any) (err error) {

	err = d.Raw().Delete(obj).Error

	return

	// if d.app.isolated {
	// 	return _isolatedCreate(obj, *d.app)
	// }

	// return d.db.Transaction(func(tx *gorm.DB) error {
	// 	return _isolatedCreate(obj, d.app.isolateDatabase(tx))
	// })
}

func SortAndFindAllWhere[T any, CtxType any](db DbWrapper[CtxType], sortByField string, sortBy int, limit, offset int, where string, whereArgs ...any) typed.Result[[]T] {

	result := []T{}

	sortOrder := "ASC"
	if sortBy == -1 {
		sortOrder = "DESC"
	}

	dbQ := db.Raw()

	if limit > 0 {
		dbQ = dbQ.Limit(limit)
	}

	if offset > 0 {
		dbQ = dbQ.Offset(offset)
	}

	if sortByField != "" {
		sortQValue := fmt.Sprintf("%s %s", sortByField, sortOrder)
		dbQ = dbQ.Order(sortQValue)
	}

	qB := dbQ.Where(where, whereArgs...)

	findErr := qB.Find(&result).Error

	if findErr != nil {
		return typed.ResultFailed[[]T](findErr)
	} else {
		return typed.ResultOk(result)
	}

}

func FindAllWhere[T any, CtxType any](db DbWrapper[CtxType], where string, whereArgs ...any) typed.Result[[]T] {

	result := []T{}

	findErr := db.Raw().Where(where, whereArgs...).Find(&result).Error

	if findErr != nil {
		return typed.ResultFailed[[]T](findErr)
	} else {
		return typed.ResultOk(result)
	}

}

func FindAndLockFirstWhere[T any, CtxType any](db DbWrapper[CtxType], where string, whereArgs ...any) typed.Result[T] {

	var result T

	findErr := db.Raw().Clauses(clause.Locking{
		Strength: "UPDATE",
	}).Where(where, whereArgs...).First(&result).Error

	if findErr != nil {
		return typed.ResultFailed[T](findErr)
	} else {
		return typed.ResultOk(result)
	}

}

func FindFirstWhere[T any, CtxType any](db DbWrapper[CtxType], where string, whereArgs ...any) typed.Result[T] {

	var result T

	findErr := db.Raw().Where(where, whereArgs...).First(&result).Error

	if findErr != nil {
		return typed.ResultFailed[T](findErr)
	} else {
		return typed.ResultOk(result)
	}

}
