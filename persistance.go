package simpleapi

import (
	"fmt"

	typed "github.com/cldfn/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BeforeCreateCbAware interface {
	BeforeEntityCreate(ctx *AppContext) error
}

type AfterCreateCbAware interface {
	AfterEntityCreate(ctx *AppContext) error
}

type OnAfterUpdateCbAware interface {
	AfterUpdate(ctx *AppContext) error
}

type OnBeforeUpdateCbAware interface {
	BeforeUpdate(ctx *AppContext) error
}

type OnUpdateEventHandler[T any] interface {
	OnUpdate(ctx *AppContext, prevState T, permission RequestData) error
}

type DbWrapper struct {
	db    *gorm.DB
	topDb *gorm.DB

	app         *AppContext
	debug       bool
	automigrate bool
}

func (d DbWrapper) Automigrate(v bool) DbWrapper {
	d.automigrate = v
	return d
}

func WrapGormDb(d *gorm.DB, ctx *AppContext) DbWrapper {
	return DbWrapper{
		db:          d,
		topDb:       d,
		app:         ctx,
		debug:       false,
		automigrate: true,
	}
}

func (d DbWrapper) Raw() *gorm.DB {

	var dbRef *gorm.DB

	if d.debug {
		dbRef = d.db.Debug()
	} else {
		dbRef = d.db
	}

	return dbRef
}

func (d *DbWrapper) Debug(v bool) {
	d.debug = v
}
func (d DbWrapper) CleanCopy() DbWrapper {
	return WrapGormDb(d.topDb, d.app)
}

func (d *DbWrapper) setRaw(_db *gorm.DB) {
	d.db = _db
}

func _isolatedCreate(obj any, ctx AppContext) (err error) {

	_db := ctx.Db.Raw()

	__obj, ok := obj.(OnBeforeUpdateCbAware)
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
	_obj, ok := obj.(AfterCreateCbAware)
	if ok {
		return _obj.AfterEntityCreate(&ctx)
	}

	return nil
}

func _isolatedSaveOnlyFields(obj any, ctx AppContext, fields []string) (err error) {

	_db := ctx.Db.Raw()

	__obj, ok := obj.(OnBeforeUpdateCbAware)
	if ok {
		beforeUpdateCbErr := __obj.BeforeUpdate(&ctx)
		if beforeUpdateCbErr != nil {
			return beforeUpdateCbErr
		}
	}

	if len(fields) > 0 {
		err = _db.Select(fields).Updates(obj).Error

	} else {
		err = _db.Save(obj).Error
	}

	if err != nil {
		return err
	}

	// check after event
	// should be executed after transaction commit
	_obj, ok := obj.(OnAfterUpdateCbAware)
	if ok {
		return _obj.AfterUpdate(&ctx)
	}

	return nil
}

func _isolatedSave(obj any, ctx AppContext) (err error) {

	_db := ctx.Db.Raw()

	__obj, ok := obj.(OnBeforeUpdateCbAware)
	if ok {
		beforeUpdateCbErr := __obj.BeforeUpdate(&ctx)
		if beforeUpdateCbErr != nil {
			return beforeUpdateCbErr
		}
	}

	err = _db.Save(obj).Error

	if err != nil {
		return err
	}

	// check after event
	// should be executed after transaction commit
	_obj, ok := obj.(OnAfterUpdateCbAware)
	if ok {
		return _obj.AfterUpdate(&ctx)
	}

	return nil
}

func (d DbWrapper) UpdateFields(obj any, fields ...string) (err error) {

	if d.app.isolated {
		return _isolatedSaveOnlyFields(obj, *d.app, fields)
	}

	return d.db.Transaction(func(tx *gorm.DB) error {
		return _isolatedSaveOnlyFields(obj, d.app.isolateDatabase(tx), fields)
	})
}

func (d DbWrapper) Save(obj any) (err error) {

	if d.app.isolated {
		return _isolatedSave(obj, *d.app)
	}

	return d.db.Transaction(func(tx *gorm.DB) error {
		return _isolatedSave(obj, d.app.isolateDatabase(tx))
	})
}

func (d DbWrapper) Create(obj any) (err error) {

	if d.app.isolated {
		return _isolatedCreate(obj, *d.app)
	}

	return d.db.Transaction(func(tx *gorm.DB) error {
		return _isolatedCreate(obj, d.app.isolateDatabase(tx))
	})
}

func (d DbWrapper) Delete(obj any) (err error) {

	err = d.Raw().Delete(obj).Error

	return

	// if d.app.isolated {
	// 	return _isolatedCreate(obj, *d.app)
	// }

	// return d.db.Transaction(func(tx *gorm.DB) error {
	// 	return _isolatedCreate(obj, d.app.isolateDatabase(tx))
	// })
}

func SortAndFindAllWhere[T any](db DbWrapper, sortByField string, sortBy int, limit, offset int, where string, whereArgs ...any) typed.Result[[]T] {

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

func FindAllWhere[T any](db DbWrapper, where string, whereArgs ...any) typed.Result[[]T] {

	result := []T{}

	findErr := db.Raw().Where(where, whereArgs...).Find(&result).Error

	if findErr != nil {
		return typed.ResultFailed[[]T](findErr)
	} else {
		return typed.ResultOk(result)
	}

}

func FindAndLockFirstWhere[T any](db DbWrapper, where string, whereArgs ...any) typed.Result[T] {

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

func FindFirstWhere[T any](db DbWrapper, where string, whereArgs ...any) typed.Result[T] {

	var result T

	findErr := db.Raw().Where(where, whereArgs...).First(&result).Error

	if findErr != nil {
		return typed.ResultFailed[T](findErr)
	} else {
		return typed.ResultOk(result)
	}

}
