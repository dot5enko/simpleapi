package simpleapi

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

var (
	ErrNoRightToPerformAction = fmt.Errorf("no rights to perform action")
	ErrObjectNotFound         = fmt.Errorf("object not found")
)

type CrudContext[T any, CtxType any] struct {
	Crud *CrudConfig[T, CtxType]
	App  *AppContext[CtxType]
}

type ModelIdFetcher[T any] func(obj *T) uint64
type RelatedObjectIdGetter[T any] func(obj *T) uint64
type RelatedItemsFetcher[OfType any, RelatedType any] func(ctx *gin.Context)

type CrudConfig[T any, CtxType any] struct {
	ParentGroup *gin.RouterGroup
	Model       T
	App         *AppContext[CtxType]

	existing     []gin.HandlerFunc
	beforeCreate []gin.HandlerFunc

	objectCreate func(ctx CrudContext[T, CtxType], obj *T) error
	afterCreate  func(ctx *AppContext[CtxType], obj *T) error

	permTable                 TblName
	permRelatedObjectIdGetter RelatedObjectIdGetter[T]

	relTypeTable string

	hasMultiple []ApiObjectRelation[T, CtxType]

	passObject bool
}

func (it CrudConfig[T, CtxType]) RelTable() string {
	return it.relTypeTable
}

type TblName interface {
	TableName() string
}

func (it *CrudConfig[T, CtxType]) StoreRelation(reltable TblName) *CrudConfig[T, CtxType] {
	it.relTypeTable = reltable.TableName()
	return it
}

func (it *CrudConfig[T, CtxType]) StoreObjectInContext() *CrudConfig[T, CtxType] {
	it.passObject = true
	return it
}

func (it *CrudConfig[T, CtxType]) OnObjectCreate(h func(crudContext CrudContext[T, CtxType], obj *T) error) *CrudConfig[T, CtxType] {
	it.objectCreate = h
	return it
}

type RelatedObjectHandlerInit[T any, CtxType any] func(appctx *AppContext[CtxType], config *ApiObjectRelation[T, CtxType])

type ApiObjectRelation[RelatedToType any, CtxType any] struct {
	PathSuffix          string
	ParentObjectIdField string
	ItemHandler         RelatedObjectHandlerInit[RelatedToType, CtxType]
}

func (it *CrudConfig[T, CtxType]) HasMultiple(relations ...ApiObjectRelation[T, CtxType]) *CrudConfig[T, CtxType] {

	it.hasMultiple = relations

	return it
}

func (it *CrudConfig[T, CtxType]) PermissionTable(relationsTable TblName, relatedIdGetter RelatedObjectIdGetter[T]) *CrudConfig[T, CtxType] {

	it.permTable = relationsTable
	it.permRelatedObjectIdGetter = relatedIdGetter

	return it
}

func (it *CrudConfig[T, CtxType]) OnAfterCreate(h func(appctx *AppContext[CtxType], obj *T) error) *CrudConfig[T, CtxType] {
	it.afterCreate = h
	return it
}

func (it *CrudConfig[T, CtxType]) UseExisting(h ...gin.HandlerFunc) *CrudConfig[T, CtxType] {

	it.existing = h
	return it
}

func (it *CrudConfig[T, CtxType]) UseBeforeCreate(h ...gin.HandlerFunc) *CrudConfig[T, CtxType] {
	it.beforeCreate = h
	return it
}

func storeObjectInContext[T any, CtxType any](appctx *AppContext[CtxType], ctx *gin.Context) Result[T] {
	orgId := GetArgUint(ctx, "id")

	var obj T
	err := appctx.Db.Where("id = ?", orgId).First(&obj).Error

	if err != nil {
		return ResultFailed[T](err)
	} else {
		ctx.Set("_object", obj)

		return ResultOk(obj)
	}
}

func createRelAfterSave[T any, CtxType any](appctx *AppContext[CtxType], obj *T, relTable string) error {

	var relation UserToObject

	// todo optimize
	val := reflect.Indirect(reflect.ValueOf(obj))

	relation.ObjectId = val.FieldByName("Id").Uint()
	relation.UserId = GetUserId(appctx.Request)

	return appctx.Db.Table(relTable).Create(&relation).Error
}

// returns -1 if no role attached
// user has no access to object
func UserRelationRole[T any](
	appctx *AppContext[T],
	objectId uint64,
	userId uint64,
	reltable TblName,
	role uint8,
) int8 {

	var relationInfo UserToObject

	err := appctx.Db.Table(reltable.TableName()).
		Where("object_id = ? and user_id = ? ", objectId, userId).
		First(&relationInfo).
		Error

	if err != nil {
		return -1
	}

	return int8(relationInfo.Role)
}

func getIdValue[T any](obj T) uint64 {
	val := reflect.Indirect(reflect.ValueOf(obj))
	return val.FieldByName("Id").Uint()
}

func checkRole[T any, CtxT any](appctx *AppContext[CtxT], ctx *gin.Context, relatedTable string) bool {

	objContext := MustGetObjectFromContext[T](ctx, "_object")
	userId := GetUserId(ctx)

	// todo optimize
	objectId := getIdValue(objContext)

	var relationInfo UserToObject

	err := appctx.Db.Table(relatedTable).
		Where("object_id = ? and user_id = ? ", objectId, userId).
		First(&relationInfo).Error

	if err != nil {
		return false
	} else {

		// check if user has proper access rights ?
		// or do this later in specific action ?
		ctx.Set("_role", relationInfo)
		return true
	}
}

func toDto[T any, CtxType any](it T, appctx *AppContext[CtxType], permission int) Result[interface{}] {
	dtoPresenter, ok := any(it).(ApiDto[CtxType])
	if ok {

		_dtoResult := dtoPresenter.ToApiDto(permission, appctx)

		if _dtoResult.IsOk() {
			return ResultOk[interface{}](_dtoResult.Unwrap())
		} else {
			return ResultFailed[interface{}](_dtoResult.UnwrapError())
		}
	} else {
		return ResultOk[interface{}](it)
	}
}

func CheckRights[T any, CtxType any](
	crudContext CrudContext[T, CtxType],
	obj *T,
	relatedIdGetter RelatedObjectIdGetter[T],
	tbName TblName,
) error {

	appctx := crudContext.App

	if appctx.Request == nil {
		log.Printf("checking rights on empty request")
	}

	userId := GetUserId(appctx.Request)
	relatedId := relatedIdGetter(obj)

	minRoleRequired := 0

	log.Printf("Checking rights for related %d, for user %d. related object : %#+v", relatedId, userId, obj)

	userRole := UserRelationRole(appctx, relatedId, userId, tbName, uint8(minRoleRequired))

	if userRole >= 0 {
		return nil
	} else {
		return ErrNoRightToPerformAction
	}
}

func New[T any, CtxType any](appctx *AppContext[CtxType], group *gin.RouterGroup, model T) *CrudConfig[T, CtxType] {

	result := CrudConfig[T, CtxType]{
		ParentGroup: group,
		Model:       model,
		App:         appctx,
	}

	// todo check rights in all methods
	// todo recover in userland hooks

	return &result
}

func (result *CrudConfig[T, CtxType]) Generate() *CrudConfig[T, CtxType] {

	group := result.ParentGroup
	model := result.Model
	appctx := result.App

	if result.beforeCreate != nil {
		group.Use(result.beforeCreate...)
	}

	// create
	group.POST("", func(ctx *gin.Context) {

		modelCopy := model

		// create new object
		data, err := ctx.GetRawData()
		if err != nil {
			ctx.JSON(500, gin.H{
				"msg": "unable to get object data, when creating new one",
				"err": err.Error(),
			})
			return
		}

		modelFillable, ok := any(&modelCopy).(ApiDtoFillable)

		if !ok {
			unmarshalErr := json.Unmarshal(data, &modelCopy)

			if unmarshalErr != nil {
				ctx.JSON(500, gin.H{
					"msg": "unable to decode obhect info",
					"err": unmarshalErr.Error(),
				})
				return
			}
		} else {
			parsedJson := gjson.ParseBytes(data)
			modelFillable.FromDto(parsedJson)
		}

		createdErr := appctx.Db.Transaction(func(tx *gorm.DB) error {

			isolatedContext := &AppContext[CtxType]{
				Db:      tx,
				Data:    result.App.Data,
				Request: ctx,
			}

			if true {
				// todo optimize check
				if result.permTable != nil && result.permRelatedObjectIdGetter != nil {

					appctx.Request = ctx

					c := CrudContext[T, CtxType]{
						Crud: result,
						App:  appctx,
					}

					rightsErr := CheckRights(c, &modelCopy, result.permRelatedObjectIdGetter, result.permTable)
					if rightsErr != nil {

						return rightsErr
					}
				}
			}

			if result.objectCreate != nil {

				crudCtx := CrudContext[T, CtxType]{
					App:  isolatedContext,
					Crud: result,
				}

				errCreate := result.objectCreate(crudCtx, &modelCopy)
				if errCreate != nil {
					return fmt.Errorf("unable to perform pre object create hook: %s", errCreate.Error())
				}
			}

			createErr := tx.Model(&model).Create(&modelCopy).Error

			if createErr != nil {
				return fmt.Errorf("unable to create new object: %s", createErr.Error())
			}

			if result.relTypeTable != "" {
				afterCreateErr := createRelAfterSave(isolatedContext, &modelCopy, result.relTypeTable)
				if afterCreateErr != nil {
					return fmt.Errorf("unable to create related reference: %s", afterCreateErr.Error())
				}
			}

			if result.afterCreate != nil {
				afterCreateErr := result.afterCreate(isolatedContext, &modelCopy)
				if afterCreateErr != nil {
					return fmt.Errorf("unable to perform pre object create hook: %s", afterCreateErr.Error())
				}
			}

			return nil
		})

		if createdErr != nil {
			ctx.JSON(500, gin.H{
				"msg": "unable to create new object",
				"err": createdErr.Error(),
			})
			return
		}

		ctx.JSON(200, gin.H{
			"created": true,
			"object":  toDto(modelCopy, appctx, 0).Unwrap(),
		})
	})

	// get list
	// todo impl paging
	group.GET("", func(ctx *gin.Context) {

		// if result.relTypeTable == "" {
		// 	ctx.AbortWithStatusJSON(404, gin.H{
		// 		"msg": "no such endpoint",
		// 	})
		// 	return
		// }

		userRelatedObjects := []UserToObject{}

		relatedErr := appctx.Db.
			Where("user_id = ?", GetUserId(ctx)).
			Table(result.relTypeTable).
			Find(&userRelatedObjects).
			Error

		if relatedErr != nil {
			ctx.AbortWithStatusJSON(500, gin.H{
				"msg": "unable to get related objects",
				"err": relatedErr.Error(),
			})
			return
		}

		items := []T{}

		if len(userRelatedObjects) > 0 {

			ids := []uint64{}

			for _, it := range userRelatedObjects {
				ids = append(ids, it.ObjectId)
			}

			foundErr := appctx.Db.Where("id IN ?", ids).Find(&items).Error
			if foundErr != nil {
				ctx.JSON(500, gin.H{
					"msg": "unable to find items",
					"err": foundErr.Error(),
				})
			}
		}

		dtos := []any{}
		// convert to dto objects

		for _, it := range items {

			// check if item has dto converter
			// todo pass permission value
			_dtoResult := toDto(it, appctx, 0)
			if _dtoResult.IsOk() {
				dtos = append(dtos, _dtoResult.Unwrap())
			} else {
				log.Printf("unable to convert object to api dto : %s", _dtoResult.UnwrapError().Error())
			}
		}

		ctx.JSON(200, gin.H{
			"items": dtos,
		})
	})

	existingItems := group.Group("/:id")
	existingItems.Use(func(ctx *gin.Context) {

		// todo do in compile time
		if result.passObject || result.relTypeTable != "" {
			stored := storeObjectInContext[T](appctx, ctx)

			if stored.IsOk() {

				// check permission on child of owned object
				if result.permTable != nil && result.permRelatedObjectIdGetter != nil {

					c := CrudContext[T, CtxType]{
						Crud: result,
						App:  appctx,
					}

					modelEntity := stored.Unwrap()

					rightsErr := CheckRights(c, &modelEntity, result.permRelatedObjectIdGetter, result.permTable)
					if rightsErr != nil {
						ctx.AbortWithStatusJSON(500, gin.H{
							"msg": "no permission to alter object",
							"err": rightsErr.Error(),
						})
						return
					}
				}

				// permission on owned objects
				if result.relTypeTable != "" {
					userToObjectHasPermission := checkRole[T](appctx, ctx, result.relTypeTable)
					if !userToObjectHasPermission {
						ctx.AbortWithStatusJSON(500, gin.H{
							"msg": "no permission to alter object",
							"err": stored.UnwrapError().Error(),
						})
						return
					}

				}
			} else {
				ctx.AbortWithStatusJSON(500, gin.H{
					"msg": "unable to store object in context",
					"err": stored.UnwrapError().Error(),
				})
				return
			}
		}

		if result.existing != nil {
			for _, it := range result.existing {
				if !ctx.IsAborted() {
					it(ctx)
				}
			}
		}
	})

	existingItems.PATCH("", func(ctx *gin.Context) {

		modelCopy := model

		idParam := ctx.Param("id")
		findErr := appctx.Db.Model(model).First(&modelCopy, "id = ?", idParam).Error

		if findErr != nil {
			ctx.JSON(404, gin.H{
				"msg": "object not found",
				"err": findErr.Error(),
			})
		} else {

			// parse input data
			copy2 := map[string]interface{}{}

			data, err := ctx.GetRawData()
			if err != nil {
				ctx.JSON(500, gin.H{
					"msg": "unable to get object data, when creating new one",
					"err": err.Error(),
				})
				return
			}
			unmarshalErr := json.Unmarshal(data, &copy2)

			if unmarshalErr != nil {
				ctx.JSON(500, gin.H{
					"msg": "unable to decode obhect info",
					"err": unmarshalErr.Error(),
				})
				return
			}

			panic("update is not implemented")
			// database.Fill(&modelCopy, copy2)

			saveError := appctx.Db.Save(&modelCopy).Error
			if saveError != nil {
				ctx.JSON(404, gin.H{
					"msg": "unable to update object",
					"err": saveError.Error(),
				})
			} else {
				ctx.JSON(200, gin.H{
					"item": toDto(modelCopy, appctx, 0).Unwrap(),
				})
			}
		}

	})

	existingItems.DELETE("", func(ctx *gin.Context) {

		idParam := ctx.Param("id")
		findErr := appctx.Db.Model(model).First(&model, "id = ?", idParam).Error

		if findErr != nil {
			ctx.JSON(404, gin.H{
				"msg": "object not found",
				"err": findErr.Error(),
			})
		} else {
			deleteError := appctx.Db.Delete(&model).Error
			if deleteError != nil {
				ctx.JSON(500, gin.H{
					"msg": "unable to remove",
					"err": deleteError.Error(),
				})
				return
			} else {
				ctx.JSON(200, gin.H{
					"ok": true,
				})
			}
		}

	})

	existingItems.GET("", func(ctx *gin.Context) {

		idParam := ctx.Param("id")

		modelCopy := model

		// todo validate
		errFirst := appctx.Db.Model(&model).Where("id = ?", idParam).First(&modelCopy).Error

		if errFirst != nil {
			ctx.JSON(404, gin.H{
				"msg": "object not found",
				"err": errFirst.Error(),
			})
		} else {

			// todo get role

			ctx.JSON(200, gin.H{
				"item": toDto(modelCopy, appctx, 0).Unwrap(),
			})
		}
	})

	for _, relatedItem := range result.hasMultiple {

		cur := relatedItem

		existingItems.GET("/"+cur.PathSuffix, func(ctx *gin.Context) {

			isolated := &AppContext[CtxType]{
				Db:      appctx.Db,
				Data:    result.App.Data,
				Request: ctx,
			}

			cur.ItemHandler(isolated, &cur)

		})
	}

	return result
}
