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
	OnUpdate(ctx *AppContext[CtxType], prevState T, permission RequestData) error
}

type DbWrapper[CtxType any] struct {
	db    *gorm.DB
	topDb *gorm.DB

	app         *AppContext[CtxType]
	debug       bool
	automigrate bool
}

func (d DbWrapper[CtxType]) Automigrate(v bool) DbWrapper[CtxType] {
	d.automigrate = v
	return d
}

func WrapGormDb[T any](d *gorm.DB, ctx *AppContext[T]) DbWrapper[T] {
	return DbWrapper[T]{
		db:          d,
		topDb:       d,
		app:         ctx,
		debug:       false,
		automigrate: true,
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

	var dbRef *gorm.DB

	if ctx.Db.debug {
		dbRef = _db.Debug()
	} else {
		dbRef = _db
	}

	err = dbRef.Create(obj).Error

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

func _isolatedSaveOnlyFields[CtxType any](obj any, ctx AppContext[CtxType], fields []string) (err error) {

	_db := ctx.Db.Raw()

	__obj, ok := obj.(OnBeforeUpdateCbAware[CtxType])
	if ok {
		beforeUpdateCbErr := __obj.BeforeUpdate(&ctx)
		if beforeUpdateCbErr != nil {
			return beforeUpdateCbErr
		}
	}

	var dbRef *gorm.DB

	if ctx.Db.debug {
		dbRef = _db.Debug()
	} else {
		dbRef = _db
	}

	if len(fields) > 0 {
		err = dbRef.Select(fields).Updates(obj).Error

	} else {
		err = _db.Save(obj).Error
	}

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

func _isolatedSave[CtxType any](obj any, ctx AppContext[CtxType]) (err error) {

	_db := ctx.Db.Raw()

	__obj, ok := obj.(OnBeforeUpdateCbAware[CtxType])
	if ok {
		beforeUpdateCbErr := __obj.BeforeUpdate(&ctx)
		if beforeUpdateCbErr != nil {
			return beforeUpdateCbErr
		}
	}

	var dbRef *gorm.DB

	if ctx.Db.debug {
		dbRef = _db.Debug()
	} else {
		dbRef = _db
	}

	err = dbRef.Save(obj).Error

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

func (d DbWrapper[CtxType]) UpdateFields(obj any, fields ...string) (err error) {

	if d.app.isolated {
		return _isolatedSaveOnlyFields(obj, *d.app, fields)
	}

	return d.db.Transaction(func(tx *gorm.DB) error {
		return _isolatedSaveOnlyFields(obj, d.app.isolateDatabase(tx), fields)
	})
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
