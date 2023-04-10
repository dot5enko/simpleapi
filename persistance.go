package simpleapi

import "gorm.io/gorm"

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

type DbWrapper[CtxType any] struct {
	db  *gorm.DB
	app *AppContext[CtxType]
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

func FindAllWhere[T any, CtxType any](db DbWrapper[CtxType], where string) Result[[]T] {

	result := []T{}

	findErr := db.Raw().Where(where).Find(&result).Error

	if findErr != nil {
		return ResultFailed[[]T](findErr)
	} else {
		return ResultOk(result)
	}

}

func FindFirstWhere[T any, CtxType any](db DbWrapper[CtxType], where string) Result[T] {

	var result T

	findErr := db.Raw().Where(where).First(&result).Error

	if findErr != nil {
		return ResultFailed[T](findErr)
	} else {
		return ResultOk(result)
	}

}