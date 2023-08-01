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

func (it *CrudConfig[T, CtxType]) PermissionTable(relationsTable TblName, relatedIdGetter RelatedObjectIdGetter[T], field_name ...string) *CrudConfig[T, CtxType] {

	it.permTable = relationsTable

	// trick``
	if len(field_name) > 0 {
		it.permFieldName = field_name[0]
	} else {
		it.permFieldName = ""
	}

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

func storeObjectInContext[T any, CtxType any](appctx *AppContext[CtxType], ctx *gin.Context) typed.Result[T] {
	orgId := GetArgUint(ctx, "id")

	result := FindFirstWhere[T](appctx.Db, "id = ?", orgId)

	if result.IsOk() {
		ctx.Set("_object", result.Unwrap())
	}

	return result
}

func createRelAfterSave[T any, CtxType any](appctx *AppContext[CtxType], obj *T, relTable string) error {

	var relation UserToObject

	// todo optimize
	val := reflect.Indirect(reflect.ValueOf(obj))

	relation.ObjectId = val.FieldByName("Id").Uint()
	relation.UserId = GetUserId(appctx.Request)

	return appctx.Db.Raw().Table(relTable).Create(&relation).Error
}

// returns -1 if no role attached
// user has no access to object
func UserRelationRole[T any](
	appctx *AppContext[T],
	objectId any,
	userId uint64,
	reltable TblName,
	role uint8,
) int8 {

	var relationInfo UserToObject

	err := appctx.Db.Raw().Table(reltable.TableName()).
		Where("object_id = ? and user_id = ? ", objectId, userId).
		First(&relationInfo).
		Error

	if err != nil {
		return -1
	}

	return int8(relationInfo.Role)
}

// todo add role
func GetUserRelatedObjects[T any](
	appctx *AppContext[T],
	reltable TblName,
) []uint64 {

	userId := GetUserId(appctx.Request)

	var relationInfo []UserToObject

	err := appctx.Db.Raw().Table(reltable.TableName()).
		Where("user_id = ? ", userId).
		Find(&relationInfo).
		Error

	if err != nil {

		log.Printf("unable to get user related objects: %s", err.Error())

		return nil
	}

	result := []uint64{}

	for _, relInfo := range relationInfo {
		result = append(result, relInfo.ObjectId)
	}

	return result
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

	err := appctx.Db.Raw().Table(relatedTable).
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

func ToDto[T any, CtxType any](it T, appctx *AppContext[CtxType], permission int) typed.Result[map[string]any] {

	m := appctx.Db.ApiData(it)

	rawDto := m.ToDto(it)

	if m.OutExtraMethod {

		dtoPresenter, _ := any(it).(ApiDto[CtxType])
		return dtoPresenter.ToApiDto(rawDto, permission, appctx)
	} else {
		return typed.ResultOk(rawDto)
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

	userRole := UserRelationRole(appctx, relatedId, userId, tbName, uint8(minRoleRequired))

	if userRole >= 0 {
		return nil
	} else {
		return ErrNoRightToPerformAction
	}
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
				"msg": "No write permission",
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
		SortField string `json:"sort_field"`
		SortOrder int    `json:"order"`
		Page      int    `json:"page"`
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

		// filters
		// todo make it secure!
		{
			parts := []string{}

			// decode query
			filtersMap := map[string]any{}
			json.Unmarshal([]byte(ctx.Query("filter")), &filtersMap)

			for filterFieldName, filterValue := range filtersMap {

				// allow only whitelisted fields

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
		appctx.Db.Raw().Model(&modelObj).Count(&totalItems)
		pagesCount := math.Ceil(float64(totalItems) / float64(result.paging.PerPage))

		// todo add paging
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
