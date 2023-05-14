package simpleapi

import (
	"github.com/dot5enko/typed"
	"gorm.io/gorm"
)

type BeforeCreateCbAware[CtxType any] interface {
	BeforeCreate(ctx *AppContext[CtxType]) error
}

type AfterCreateCbAware[CtxType any] interface {
	AfterCreate(ctx *AppContext[CtxType]) error
}

type OnAfterUpdateCbAware[CtxType any] interface {
	AfterUpdate(ctx *AppContext[CtxType]) error
}

type OnBeforeUpdateCbAware[CtxType any] interface {
	BeforeUpdate(ctx *AppContext[CtxType]) error
}

type OnUpdateEventHandler[CtxType any, T any] interface {
	OnUpdate(ctx *AppContext[CtxType], prevState T, curState T) error
}

type DbWrapper[CtxType any] struct {
	db  *gorm.DB
	app *AppContext[CtxType]
}

func WrapGormDb[T any](d *gorm.DB, ctx *AppContext[T]) DbWrapper[T] {
	return DbWrapper[T]{
		db:  d,
		app: ctx,
	}
}

func (d DbWrapper[CtxType]) Raw() *gorm.DB {
	return d.db
}

func (d *DbWrapper[CtxType]) setRaw(_db *gorm.DB) {
	d.db = _db
}

func _isolatedCreate[CtxType any](obj any, ctx AppContext[CtxType]) (err error) {

	_db := ctx.Db.Raw()

	__obj, ok := obj.(BeforeCreateCbAware[CtxType])
	if ok {
		beforeUpdateCbErr := __obj.BeforeCreate(&ctx)
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
		return _obj.AfterCreate(&ctx)
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

	err = _db.Save(obj).Error

	if err != nil {
		return err
	}

	// check after event
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

func FindAllWhere[T any, CtxType any](db DbWrapper[CtxType], where string, whereArgs ...any) typed.Result[[]T] {

	result := []T{}

	findErr := db.Raw().Where(where, whereArgs...).Find(&result).Error

	if findErr != nil {
		return typed.ResultFailed[[]T](findErr)
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
