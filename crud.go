package simpleapi

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"reflect"
	"strings"
	"time"

	"github.com/dot5enko/typed"
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

// object id could be non integer
type RelatedObjectIdGetter[T any] func(obj *T) any
type RelatedItemsFetcher[OfType any, RelatedType any] func(ctx *gin.Context)

type CrudConfig[T any, CtxType any] struct {
	ParentGroup *gin.RouterGroup
	Model       T
	App         *AppContext[CtxType]
	CrudGroup   *CrudGroup[CtxType]

	existing     []gin.HandlerFunc
	beforeCreate []gin.HandlerFunc

	objectCreate func(ctx CrudContext[T, CtxType], obj *T) error
	afterCreate  func(ctx *AppContext[CtxType], obj *T) error

	permTable                 TblName
	permFieldName             string
	permRelatedObjectIdGetter RelatedObjectIdGetter[T]

	relTypeTable string

	hasMultiple []ApiObjectRelation[T, CtxType]

	passObject bool

	disableFilterOverFields map[string]bool

	objectIdField string

	paging PagingConfig
}

type PagingConfig struct {
	PerPage int
}

func (it *CrudConfig[T, CtxType]) IdField(fieldName string) *CrudConfig[T, CtxType] {

	it.objectIdField = fieldName

	return it
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

func storeObjectInContext[T any, CtxType any](appctx *AppContext[CtxType], ctx *gin.Context) typed.Result[T] {
	orgId := GetArgUint(ctx, "id")

	result := FindFirstWhere[T](appctx.Db, "id = ?", orgId)

	if result.IsOk() {
		ctx.Set("_object", result.Unwrap())
	}

	return result
}

func getIdValue[T any](obj T) uint64 {
	val := reflect.Indirect(reflect.ValueOf(obj))
	return val.FieldByName("Id").Uint()
}

func New[T any, CtxType any](crudGroup *CrudGroup[CtxType], group *gin.RouterGroup, model T) *CrudConfig[T, CtxType] {

	result := CrudConfig[T, CtxType]{
		ParentGroup: group,
		Model:       model,
		App:         &crudGroup.Ctx,
		CrudGroup:   crudGroup,

		// todo remove
		objectIdField:           crudGroup.Config.ObjectIdFieldName,
		disableFilterOverFields: map[string]bool{},

		paging: PagingConfig{
			PerPage: 30,
		},
	}

	// todo check rights in all methods
	// todo recover in userland hooks

	return &result
}

func (result *CrudConfig[T, CtxType]) Generate() *CrudConfig[T, CtxType] {

	group := result.ParentGroup
	model := result.Model
	appctx := result.App

	var writePermissionMiddleware gin.HandlerFunc = func(ctx *gin.Context) {
		wp := result.CrudGroup.Config.WritePermission
		hasPermission := (*wp)(ctx, appctx)
		if !hasPermission {
			ctx.AbortWithStatusJSON(403, gin.H{
				"msg": "No write permission, code updated",
			})
			return
		}
	}

	// todo make at compile time
	rp := result.CrudGroup.Config.ReadPermission
	if rp != nil {
		group.Use(func(ctx *gin.Context) {
			hasPermission := (*rp)(ctx, appctx)
			if !hasPermission {
				ctx.AbortWithStatusJSON(403, gin.H{
					"msg": "No read permission",
				})
				return
			}
		})
	}

	if result.beforeCreate != nil {
		group.Use(result.beforeCreate...)
	}

	// create
	group.POST("", writePermissionMiddleware, func(ctx *gin.Context) {

		var modelCopy T

		// create new object
		data, err := ctx.GetRawData()
		if err != nil {
			ctx.JSON(500, gin.H{
				"msg": "unable to get object data, when creating new one",
				"err": err.Error(),
			})
			return
		}

		// default fill from model tags
		parsedJson := gjson.ParseBytes(data)

		fillError := appctx.FillEntityFromDto(&modelCopy, parsedJson, nil)

		if fillError != nil {
			ctx.JSON(500, gin.H{
				"msg": "can't fill object with provided data",
				"err": fillError.Error(),
			})
			return
		}

		createdErr := appctx.Db.Raw().Transaction(func(tx *gorm.DB) error {

			isolatedContext := appctx.isolateDatabase(tx)
			isolatedContext.Request = ctx

			if false { // remove code
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
					App:  &isolatedContext,
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
				afterCreateErr := createRelAfterSave(&isolatedContext, &modelCopy, result.relTypeTable)
				if afterCreateErr != nil {
					return fmt.Errorf("unable to create related reference: %s", afterCreateErr.Error())
				}
			}

			if result.afterCreate != nil {
				afterCreateErr := result.afterCreate(&isolatedContext, &modelCopy)
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
			"object":  ToDto(modelCopy, appctx, 0).Unwrap(),
		})
	})

	type ListQueryParams struct {
		SortField string `form:"sort_field"`
		SortOrder int    `form:"order"`
		Page      int    `form:"page"`
	}

	// get list
	// todo impl paging
	// todo implement filters by fields
	group.GET("", func(ctx *gin.Context) {

		var modelObj T

		var items []T
		// fieldName := "id"

		filters := ""
		filterArgs := []any{}

		hasDisabledFields := len(result.disableFilterOverFields) > 0

		listQueryParams := ListQueryParams{}
		ctx.BindQuery(&listQueryParams)

		modelDataStruct := appctx.Db.ApiData(modelObj)

		// filters
		// todo make it secure!
		{
			parts := []string{}

			// decode query
			filtersMap := map[string]any{}
			json.Unmarshal([]byte(ctx.Query("filter")), &filtersMap)

			for filterFieldName, filterValue := range filtersMap {

				// allow only whitelisted fields
				_, canBeFiltered := modelDataStruct.Filterable[filterFieldName]

				if !canBeFiltered {
					continue
				}

				// if result.CrudGroup.Config.DisableFilter
				if hasDisabledFields {
					_, disabled := result.disableFilterOverFields[filterFieldName]
					if disabled {
						continue
					}
				}

				parts = append(parts, fmt.Sprintf("%s = ?", filterFieldName))
				filterArgs = append(filterArgs, filterValue)
			}

			filters = strings.Join(parts, " AND ")
		}

		// should be auth check instead

		curPage := listQueryParams.Page
		if curPage <= 0 {
			curPage = 1
		}

		limitVal := result.paging.PerPage
		offsetVal := (curPage - 1) * result.paging.PerPage

		totalItems := int64(0)
		// todo dont count soft removed
		appctx.Db.Raw().Model(&modelObj).Where(filters, filterArgs...).Count(&totalItems)
		pagesCount := math.Ceil(float64(totalItems) / float64(result.paging.PerPage))

		// check sorting field
		if listQueryParams.SortField != "" {
			_, canBeSorted := modelDataStruct.Filterable[listQueryParams.SortField]

			if !canBeSorted {
				listQueryParams.SortField = ""
			}
		}

		SortAndFindAllWhere[T](appctx.Db,
			listQueryParams.SortField,
			listQueryParams.SortOrder,
			limitVal,
			offsetVal,
			filters,
			filterArgs...).Then(func(t *[]T) *typed.Result[[]T] {

			items = *t

			dtos := []any{}
			// convert to dto objects

			for _, it := range items {

				// check if item has dto converter
				// todo pass permission value
				_dtoResult := ToDto(it, appctx, 0)
				if _dtoResult.IsOk() {
					unwrapped := _dtoResult.Unwrap()
					dtos = append(dtos, unwrapped)
				} else {
					log.Printf("unable to convert object to api dto : %s", _dtoResult.UnwrapError().Error())
				}
			}

			ctx.JSON(200, gin.H{
				"items":       dtos,
				"pages":       pagesCount,
				"total_items": totalItems,
			})

			return nil
		}).Fail(func(e error) {
			ctx.AbortWithStatusJSON(404, gin.H{
				"msg": "db err",
				"err": e.Error(),
			})
			return
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

	existingItems.PATCH("", writePermissionMiddleware, func(ctx *gin.Context) {

		var modelCopy T

		idParam := ctx.Param("id")
		// todo cache
		q := fmt.Sprintf("%s = ?", result.objectIdField)

		findResult := FindFirstWhere[T](appctx.Db, q, idParam)

		if !findResult.IsOk() {

			findErr := findResult.UnwrapError()

			ctx.JSON(404, gin.H{
				"msg": "object not found",
				"err": findErr,
			})
		} else {

			modelCopy = findResult.Unwrap()

			data, err := ctx.GetRawData()
			if err != nil {
				ctx.JSON(500, gin.H{
					"msg": "unable to get object data, when creating new one",
					"err": err.Error(),
				})
				return
			}
			parsed := gjson.ParseBytes(data)

			if !parsed.Exists() {
				ctx.JSON(500, gin.H{
					"msg": "unable to decode obhect info",
					"raw": string(data),
				})
				return
			}

			anotherCopy := modelCopy
			ref := &anotherCopy

			fillError := appctx.FillEntityFromDto(ref, parsed, nil)

			if fillError != nil {
				ctx.JSON(500, gin.H{
					"msg": "fill object fields erorr",
					"err": fillError.Error(),
				})
				return
			}

			saveError := appctx.Db.Raw().Transaction(func(tx *gorm.DB) error {

				isolatedContext := appctx.isolateDatabase(tx)
				isolatedContext.Request = ctx

				saveErr := appctx.Db.Save(&anotherCopy)

				if saveErr == nil {

					fieldsData := appctx.Db.ApiData(ref)

					if fieldsData.UpdateExtraMethod {
						// get updater

						log.Printf("model(%s) has an update event handler", fieldsData.TypeName)

						objUpdater, _ := any(ref).(OnUpdateEventHandler[CtxType, T])
						updateEventError := objUpdater.OnUpdate(appctx, modelCopy, *ref)
						if updateEventError != nil {
							return updateEventError
						}
					}
				}

				return saveErr
			})

			if saveError != nil {
				ctx.JSON(404, gin.H{
					"msg": "unable to update object",
					"err": saveError.Error(),
				})
			} else {
				ctx.JSON(200, gin.H{
					"item": ToDto(modelCopy, appctx, 0).Unwrap(),
				})
			}
		}

	})

	existingItems.DELETE("", writePermissionMiddleware, func(ctx *gin.Context) {

		idParam := ctx.Param("id")

		q := fmt.Sprintf("%s = ?", result.objectIdField)

		findResult := FindFirstWhere[T](appctx.Db, q, idParam)

		if !findResult.IsOk() {

			findErr := findResult.UnwrapError()

			ctx.JSON(404, gin.H{
				"msg": "object not found",
				"err": findErr.Error(),
			})
		} else {

			model = findResult.Unwrap()

			deleteError := appctx.Db.Delete(&model)
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

		start := time.Now()

		idParam := ctx.Param("id")
		modelCopy := model

		q := fmt.Sprintf("%s = ?", result.objectIdField)

		findResult := FindFirstWhere[T](appctx.Db, q, idParam)

		// todo validate
		errFirst := findResult.UnwrapError()

		if errFirst != nil {
			ctx.JSON(404, gin.H{
				"msg": "object not found, ",
				"err": errFirst.Error(),
			})
		} else {

			modelCopy = findResult.Unwrap()

			// todo get role

			dur := time.Since(start)
			durFloat := float64(dur.Nanoseconds()) / 1e6

			ctx.Writer.Header().Add("Server-Timing", fmt.Sprintf("miss, app;dur=%.2f", durFloat))

			ctx.JSON(200, gin.H{
				"item": ToDto(modelCopy, appctx, 0).Unwrap(),
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

func (result *CrudConfig[T, CtxType]) PerPage(val int) *CrudConfig[T, CtxType] {
	result.paging.PerPage = val
	return result
}
