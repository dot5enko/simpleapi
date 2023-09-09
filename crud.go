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
	"github.com/google/uuid"
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

	TypeDataModel FieldsMapping

	requestDataGeneratorOverride func(g *gin.Context, ctx *AppContext[CtxType]) RequestData

	existing     []gin.HandlerFunc
	beforeCreate []gin.HandlerFunc

	objectCreate func(ctx CrudContext[T, CtxType], obj *T) error
	afterCreate  func(ctx *AppContext[CtxType], obj *T) error

	permTable                 TblName
	permFieldName             string
	permRelatedObjectIdGetter RelatedObjectIdGetter[T]

	relTypeTable string

	hasMultiple []ApiObjectRelation[T, CtxType]

	hasManyConfig []HasManyConfig[CtxType]

	passObject bool

	// not used ?
	disableFilterOverFields map[string]bool

	objectIdField string

	paging PagingConfig
}

type PagingConfig struct {
	PerPage int
}

type HasManyConfig[T any] struct {
	FilterName string

	RelTable         string
	RelCurFieldName  string
	RelDestFieldName string
	InputTransformer DataTransformer[T]
}

type DataTransformer[T any] func(ctx *AppContext[T], input any) (output any)

func (it *CrudConfig[T, CtxType]) HasManyThrough(relTable string, dest_field, cur_field, filter_name string, inputTransformer DataTransformer[CtxType]) *CrudConfig[T, CtxType] {

	curConf := HasManyConfig[CtxType]{
		FilterName:       filter_name,
		RelTable:         relTable,
		RelCurFieldName:  cur_field,
		RelDestFieldName: dest_field,
	}

	it.hasManyConfig = append(it.hasManyConfig, curConf)

	return it
}

func (it *CrudConfig[T, CtxType]) HasManyFilter(filter string) (*HasManyConfig[CtxType], bool) {

	for _, it := range it.hasManyConfig {
		if it.FilterName == filter {
			return &it, true
		}
	}

	return nil, false
}

func (it *CrudConfig[T, CtxType]) RequestData(g *gin.Context) RequestData {

	ctx := &it.CrudGroup.Ctx

	if it.requestDataGeneratorOverride != nil {
		return it.requestDataGeneratorOverride(g, ctx)
	}

	if it.CrudGroup.Config.RequestDataGenerator == nil {
		return RequestData{
			IsAdmin:          false,
			RoleGroup:        0,
			AuthorizedUserId: nil,
		}
	} else {
		generated := it.CrudGroup.Config.RequestDataGenerator(g, ctx)

		if generated.Debug {
			// init logger
			generated.init_debug_logger()
		}

		return generated
	}
}

func (it *CrudConfig[T, CtxType]) IdField(fieldName string) *CrudConfig[T, CtxType] {

	it.objectIdField = fieldName

	return it
}

func (it *CrudConfig[T, CtxType]) AdminCheck(init func(g *gin.Context, ctx *AppContext[CtxType]) RequestData) *CrudConfig[T, CtxType] {

	it.requestDataGeneratorOverride = init

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

type RelatedObjectHandlerInit[T any, CtxType any] func(appctx *AppContext[CtxType], config *ApiObjectRelation[T, CtxType], req RequestData)

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

	modelData := crudGroup.Ctx.ApiData(model)

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

		TypeDataModel: modelData,
	}

	// todo check rights in all methods
	// todo recover in userland hooks

	return &result
}

type FilterOperationHandler func(fname string, decl map[string]any) (string, any)

// return "", decl["v"]

var supportedFilters = map[string]FilterOperationHandler{
	"gt": func(fname string, decl map[string]any) (string, any) {
		return fmt.Sprintf("%s > ?", fname), decl["v"]
	},
	"lt": func(fname string, decl map[string]any) (string, any) {
		return fmt.Sprintf("%s < ?", fname), decl["v"]
	},
	"gte": func(fname string, decl map[string]any) (string, any) {
		return fmt.Sprintf("%s >= ?", fname), decl["v"]
	},
	"lte": func(fname string, decl map[string]any) (string, any) {
		return fmt.Sprintf("%s <= ?", fname), decl["v"]
	},
}

func SetListFilterHandler(fname string, h FilterOperationHandler) {
	supportedFilters[fname] = h
}

func (result *CrudConfig[T, CtxType]) Generate() *CrudConfig[T, CtxType] {

	group := result.ParentGroup
	model := result.Model
	appctx := result.App

	hasAdminOnlyFiedls := false

	for _, it := range result.TypeDataModel.Fields {
		if it.AdminOnly {
			hasAdminOnlyFiedls = true
			break
		}
	}

	if hasAdminOnlyFiedls && result.CrudGroup.Config.RequestDataGenerator == nil {

		typ := reflect.Indirect(reflect.ValueOf(result.Model)).Type()
		log.Printf("crud group type has adminOnly fields, but no rule provided on how to grant role, %#+v", typ.Name())
	}

	var writePermissionMiddleware gin.HandlerFunc = func(ctx *gin.Context) {
		wp := result.CrudGroup.Config.WritePermission
		if wp != nil {
			hasPermission := (*wp)(ctx, appctx)
			if !hasPermission {
				ctx.AbortWithStatusJSON(403, gin.H{
					"msg": "No write permission, code updated",
				})
				return
			}
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

		req := result.RequestData(ctx)

		fillError := appctx.FillEntityFromDto(result.TypeDataModel, &modelCopy, parsedJson, nil, req)

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

		reqData := result.RequestData(ctx)

		ctx.JSON(200, gin.H{
			"created": true,
			"object":  ToDto(modelCopy, appctx, reqData).Unwrap(),
		})
	})

	// get list
	group.GET("", func(ctx *gin.Context) {

		var modelObj T
		var items []T

		listQueryParams := ListQueryParams{}
		ctx.BindQuery(&listQueryParams)

		// TODO put into crud group state, no need to get it each time manually
		modelDataStruct := result.TypeDataModel
		userAuthData := result.RequestData(ctx)

		// filters
		// decode userdata from query
		filtersMap := map[string]any{}
		_filterValue := ctx.Query("filter")
		json.Unmarshal([]byte(_filterValue), &filtersMap)

		filterCompiled := prepareFilterData[T, CtxType](filtersMap, result, modelDataStruct, userAuthData, listQueryParams)

		if !filterCompiled.IsOk() {
			ctx.JSON(200, gin.H{
				"items":       []any{},
				"pages":       0,
				"total_items": 0,
				"msg":         "no access",
			})
			return
		}

		filterData := filterCompiled.Unwrap()
		filtersSql := filterData.QueryPlaceholder

		totalItems := int64(0)

		// process complex filters

		complexFiltersCount := len(filterData.ComplexFilters)
		if complexFiltersCount > 0 {
			userAuthData.log_format("processing %d complex filters ", complexFiltersCount)

			for _, it := range filterData.ComplexFilters {

				func() {

					defer func() {
						rec := recover()
						if rec != nil {
							userAuthData.log_format("unable to process complex filter for field `%s`: %s", it.fiedName, rec)
						}
					}()

					val := it.inputValue

					if it.filterData.InputTransformer != nil {
						val = it.filterData.InputTransformer(appctx, val)
					}

				}()

			}

		}

		// todo cache
		appctx.Db.Raw().Model(&modelObj).Where(filtersSql, filterData.Args...).Count(&totalItems)
		pagesCount := math.Ceil(float64(totalItems) / float64(filterData.PerPage))

		SortAndFindAllWhere[T](appctx.Db,
			listQueryParams.SortField,
			listQueryParams.SortOrder,
			filterData.Limit,
			filterData.Offset,
			filtersSql,
			filterData.Args...).Then(func(t *[]T) *typed.Result[[]T] {

			items = *t

			dtos := []any{}
			// convert to dto objects

			for _, it := range items {

				// check if item has dto converter
				// todo pass permission value
				_dtoResult := ToDto(it, appctx, userAuthData)
				if _dtoResult.IsOk() {
					unwrapped := _dtoResult.Unwrap()
					dtos = append(dtos, unwrapped)
				} else {
					log.Printf("unable to convert object(%#+v) to api dto : %s", it, _dtoResult.UnwrapError().Error())
				}
			}

			if userAuthData.Debug {
				ctx.JSON(200, gin.H{
					"items":       dtos,
					"pages":       pagesCount,
					"total_items": totalItems,
					"logs":        userAuthData.getDebugLogs(),
				})
			} else {
				ctx.JSON(200, gin.H{
					"items":       dtos,
					"pages":       pagesCount,
					"total_items": totalItems,
				})
			}

			return nil
		}).Fail(func(e error) {

			eId := uuid.NewString()

			log.Printf("db err : %s: %s", eId, e.Error())

			ctx.AbortWithStatusJSON(404, gin.H{
				"msg": "db err",
				"id":  eId,
			})
			return
		})

	})

	existingItems := group.Group("/:id")
	existingItems.Use(func(ctx *gin.Context) {

		modelInfo := result.TypeDataModel

		reqData := result.RequestData(ctx)

		idParam := ctx.Param("id")

		filter := map[string]any{
			result.objectIdField: idParam,
		}

		if modelInfo.SoftDeleteField.Has {
			// do not display removed items for non admins
			if !reqData.IsAdmin {
				filter[modelInfo.SoftDeleteField.TableColumnName] = false
			} else {
				filtersMap := map[string]any{}
				_filterValue := ctx.Query("filter")
				json.Unmarshal([]byte(_filterValue), &filtersMap)

				// if admin request forcely wants to query `removed` data - no problem
				_, removeFilterExists := filtersMap[modelInfo.SoftDeleteField.FillName]
				if !removeFilterExists {
					// hide removed elements by default
					filtersMap[modelInfo.SoftDeleteField.FillName] = false
				}
			}
		}

		// todo move to compile time
		// eg generate Use(...) without ifs
		if modelInfo.UserReferenceField.Has && !reqData.IsAdmin {

			userId := reqData.AuthorizedUserId

			if userId == nil {
				ctx.AbortWithStatusJSON(404, gin.H{
					"msg": "item not f0und",
				})
				return
			} else {

				// put user reference into filter
				filter[modelInfo.UserReferenceField.TableColumnName] = userId
			}
		}

		// build filters
		// todo cache query

		filterArgs := []any{}
		filterEntries := []string{}

		for fName, fVal := range filter {
			filterEntries = append(filterEntries, fmt.Sprintf("%s = ?", fName))
			filterArgs = append(filterArgs, fVal)
		}

		// validate filter ?

		filterStr := strings.Join(filterEntries, " AND ")

		findResult := FindFirstWhere[T](appctx.Db, filterStr, filterArgs...)

		if !findResult.IsOk() {

			findErr := findResult.UnwrapError()

			eId := uuid.NewString()

			log.Printf("db err : %s: %s", eId, findErr.Error())

			ctx.AbortWithStatusJSON(404, gin.H{
				"msg": "object not found",
				"id":  eId,
			})
		} else {
			modelCopy := findResult.Unwrap()
			ctx.Set("_eobj", modelCopy)
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

		model, _ := ctx.Get("_eobj")
		modelCopy = model.(T)

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
				"msg": "unable to decode object info",
				"raw": string(data),
			})
			return
		}

		anotherCopy := modelCopy
		ref := &anotherCopy

		req := result.RequestData(ctx)

		var debugLogs *arrayLogger

		fillError := appctx.FillEntityFromDto(result.TypeDataModel, ref, parsed, nil, req)

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

			saveErr := isolatedContext.Db.Save(ref)

			// todo remove
			if saveErr == nil {

				fieldsData := result.TypeDataModel

				req.log(func(logger *log.Logger) {
					logger.Printf("saved succesfully")
				})

				if fieldsData.UpdateExtraMethod {

					req.log(func(logger *log.Logger) {
						logger.Printf("processing extra update method for entity")
					})

					objUpdater, _ := any(ref).(OnUpdateEventHandler[CtxType, T])
					updateEventError := objUpdater.OnUpdate(&isolatedContext, modelCopy)
					if updateEventError != nil {

						req.log(func(logger *log.Logger) {
							logger.Printf("rollback update due to OnUpdate: %s", updateEventError.Error())
						})

						return updateEventError
					}
				}
			} else {

				req.log(func(logger *log.Logger) {
					logger.Printf("got an error while saving item")
				})

			}

			return saveErr
		})

		if saveError != nil {

			repsJson := gin.H{
				"msg": "unable to update object",
			}

			if req.Debug {
				repsJson["err"] = saveError.Error()

				panickedErr, ok := saveError.(typed.PanickedError)
				if ok {
					repsJson["stack"] = panickedErr.Cause
				}

				repsJson["logs"] = debugLogs.lines
			}
			ctx.JSON(500, repsJson)
		} else {

			resultItem := ToDto(anotherCopy, appctx, req).Unwrap()

			_resultJson := gin.H{
				"item": resultItem,
			}

			if req.Debug {
				_resultJson["logs"] = debugLogs.lines
			}

			ctx.JSON(200, _resultJson)
		}

	})

	existingItems.DELETE("", writePermissionMiddleware, func(ctx *gin.Context) {

		var modelCopy T

		model, _ := ctx.Get("_eobj")
		modelCopy = model.(T)

		reqData := result.RequestData(ctx)

		responseData := map[string]any{}
		responseHttpCode := 200

		defer func() {

			if reqData.Debug {
				responseData["logs"] = reqData.getDebugLogs()
			}

			ctx.JSON(responseHttpCode, responseData)
		}()

		if result.TypeDataModel.SoftDeleteField.Has {
			// soft removable items are not actually deleted

			// todo make it somewhat clear what is going on here
			fname := result.TypeDataModel.SoftDeleteField.FillName
			dto := gjson.Parse(fmt.Sprintf(`{"%s":true}`, fname))

			reqData.log(func(logger *log.Logger) {
				logger.Printf("filling soft removed field : %s", fname)
			})

			fillError := appctx.FillEntityFromDto(result.TypeDataModel, &modelCopy, dto, nil, reqData)

			if fillError != nil {

				responseData["msg"] = "fill object fields error"

				if reqData.IsAdmin {
					responseData["err"] = fillError.Error()
				}

				responseHttpCode = 500

				return
			}

			updateErr := appctx.Db.Save(&modelCopy)
			if updateErr != nil {

				responseHttpCode = 500

				responseData["msg"] = "unable to soft remove"

				if reqData.IsAdmin {
					responseData["err"] = updateErr.Error()
				}

				return
			} else {

				responseHttpCode = 200

				responseData["ok"] = true
				responseData["msg"] = "soft removed"

				return
			}

		} else {

			deleteError := appctx.Db.Delete(&modelCopy)
			if deleteError != nil {

				responseHttpCode = 500

				responseData["msg"] = "unable to soft remove"

				if reqData.IsAdmin {
					responseData["err"] = deleteError.Error()
				}

				return
			} else {
				responseHttpCode = 200
				responseData["ok"] = true
			}
		}

	})

	existingItems.GET("", func(ctx *gin.Context) {

		reqData := result.RequestData(ctx)

		start := time.Now()

		var modelCopy T

		model, _ := ctx.Get("_eobj")
		modelCopy = model.(T)

		// todo get role

		dur := time.Since(start)
		durFloat := float64(dur.Nanoseconds()) / 1e6

		ctx.Writer.Header().Add("Server-Timing", fmt.Sprintf("miss, app;dur=%.2f", durFloat))

		ctx.JSON(200, gin.H{
			"item": ToDto(modelCopy, appctx, reqData).Unwrap(),
		})
	})

	for _, relatedItem := range result.hasMultiple {

		cur := relatedItem

		existingItems.GET("/"+cur.PathSuffix, func(ctx *gin.Context) {

			isolated := &AppContext[CtxType]{
				Db:      appctx.Db,
				Data:    result.App.Data,
				Request: ctx,
			}

			cur.ItemHandler(isolated, &cur, result.RequestData(ctx))

		})
	}

	return result
}

func (result *CrudConfig[T, CtxType]) PerPage(val int) *CrudConfig[T, CtxType] {
	result.paging.PerPage = val
	return result
}
