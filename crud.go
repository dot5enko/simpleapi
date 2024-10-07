package simpleapi

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/dot5enko/typed"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"gorm.io/gorm/schema"
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

type HM = map[string]any

type CrudConfig[T any, CtxType any] struct {
	ParentGroup *gin.RouterGroup
	Model       T
	App         *AppContext[CtxType]
	CrudGroup   *CrudGroup[CtxType]

	disableEndpoints EndpointsDisableConfig

	tableName       string
	primaryIdDbName string

	TypeDataModel FieldsMapping

	requestDataGeneratorOverride func(g *gin.Context, ctx *AppContext[CtxType]) RequestData

	existing     []gin.HandlerFunc
	beforeCreate []gin.HandlerFunc

	objectCreate func(ctx CrudContext[T, CtxType], obj *T) error
	afterCreate  func(ctx *AppContext[CtxType], obj *T) error

	relTypeTable string

	hasMultiple []ApiObjectRelation[T, CtxType]

	hasManyConfig []HasManyConfig[CtxType]

	passObject bool

	// not used ?
	disableFilterOverFields map[string]bool

	objectIdField string

	paging PagingConfig

	predefinedQueries map[string]predefinedQuery
}

func (it *CrudConfig[T, CtxType]) TableName() string {
	return it.tableName
}

func (it *CrudConfig[T, CtxType]) IdFieldName() string {
	return it.primaryIdDbName
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

func (it *CrudConfig[T, CtxType]) FieldFilter(filter_name string, relTable string, dest_field, cur_field string, inputTransformer DataTransformer[CtxType]) *CrudConfig[T, CtxType] {

	curConf := HasManyConfig[CtxType]{
		FilterName:       filter_name,
		RelTable:         relTable,
		RelCurFieldName:  cur_field,
		RelDestFieldName: dest_field,
		InputTransformer: inputTransformer,
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

type EndpointsDisableConfig struct {
	List   bool
	Create bool
	Get    bool
	Update bool
	Delete bool
}

func (it *CrudConfig[T, CtxType]) Disable(config EndpointsDisableConfig) *CrudConfig[T, CtxType] {
	it.disableEndpoints = config
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

func New[T any, CtxType any](crudGroup *CrudGroup[CtxType], group *gin.RouterGroup, model T) *CrudConfig[T, CtxType] {

	modelData := crudGroup.Ctx.ApiData(model)

	// this is done only on setup, so we dont care on resources and speed here
	tableInfo, _ := schema.Parse(&model, &sync.Map{}, schema.NamingStrategy{})

	if len(tableInfo.PrimaryFields) != 1 {
		panic("there should be exactly one primary key per entity. other states not supported :(")
	}

	primaryField := tableInfo.PrimaryFields[0]

	result := CrudConfig[T, CtxType]{
		ParentGroup: group,
		Model:       model,
		App:         &crudGroup.Ctx,
		CrudGroup:   crudGroup,

		tableName:       tableInfo.Table,
		primaryIdDbName: primaryField.DBName,

		// todo remove
		objectIdField:           crudGroup.Config.ObjectIdFieldName,
		disableFilterOverFields: map[string]bool{},

		paging: PagingConfig{
			PerPage: 30,
		},

		TypeDataModel: modelData,

		predefinedQueries: map[string]predefinedQuery{},
	}

	// todo check rights in all methods
	// todo recover in userland hooks

	return &result
}

type FilterOperationHandler = func(fname string, decl map[string]any) (string, any)

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
	"ne": func(fname string, decl map[string]any) (string, any) {
		return fmt.Sprintf("%s != ?", fname), decl["v"]
	},
	"in": func(fname string, decl map[string]any) (string, any) {
		return fmt.Sprintf("%s IN ?", fname), decl["v"]
	},
	"lookup": func(fname string, decl map[string]any) (string, any) {
		return fmt.Sprintf("%s LIKE ?", fname), fmt.Sprintf("%%%s%%", decl["v"])
	},
}

func SetListFilterHandler(fname string, h FilterOperationHandler) {
	supportedFilters[fname] = h
}

func (result *CrudConfig[T, CtxType]) DeleteEntity(appctx *AppContext[CtxType], modelCopy T, reqData RequestData) (respData *RespErr) {

	respData = &RespErr{
		Data: map[string]any{},
	}

	responseData := respData.Data

	if result.TypeDataModel.SoftDeleteField.Has {
		// soft removable items are not actually deleted

		// todo make it somewhat clear what is going on here
		fname := result.TypeDataModel.SoftDeleteField.FillName
		dto := gjson.Parse(fmt.Sprintf(`{"%s":1}`, fname))

		reqData.log_format("filling soft removed field : %s", fname)

		fillError := appctx.FillEntityFromDto(result.TypeDataModel, &modelCopy, dto, nil, reqData)

		if fillError != nil {

			responseData["msg"] = "fill object fields error"

			if reqData.IsAdmin {
				responseData["err"] = fillError.Error()
			}

			respData.Httpcode = 500

			return
		}

		// update only selected fields
		updateErr := appctx.Db.UpdateFields(&modelCopy, result.TypeDataModel.SoftDeleteField.TableColumnName)
		if updateErr != nil {

			respData.Httpcode = 500

			responseData["msg"] = "unable to soft remove"

			if reqData.IsAdmin {
				responseData["err"] = updateErr.Error()
			}

			return
		} else {

			respData.Httpcode = 200

			responseData["ok"] = true
			responseData["msg"] = "soft removed"

			return
		}

	} else {

		deleteError := appctx.Db.Delete(&modelCopy)
		if deleteError != nil {

			respData.Httpcode = 500

			responseData["msg"] = "unable to soft remove"

			if reqData.IsAdmin {
				responseData["err"] = deleteError.Error()
			}

			return
		} else {
			respData.Httpcode = 200
			responseData["ok"] = true
		}
	}

	return
}

func (result *CrudConfig[T, CtxType]) CreateEntity(appctx *AppContext[CtxType], ctx *gin.Context, parsedJson gjson.Result, reqData RequestData) (objectCreated T, respData *RespErr) {
	var modelCopy T

	fillError := appctx.FillEntityFromDto(result.TypeDataModel, &modelCopy, parsedJson, nil, reqData)

	if fillError != nil {
		respData = NewRespErr(500, HM{
			"msg": "can't fill object with provided data",
			"err": fillError.Error(),
		})
		return
	}

	createdErr := appctx.DbTransaction(func(isolatedContext AppContext[CtxType]) error {

		// todo check if object is used somewhere
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

		createErr := isolatedContext.Db.Create(&modelCopy)

		if createErr != nil {
			return fmt.Errorf("unable to create new object: %s", createErr.Error())
		}

		// if result.relTypeTable != "" {
		// 	afterCreateErr := createRelAfterSave(&isolatedContext, &modelCopy, result.relTypeTable)
		// 	if afterCreateErr != nil {
		// 		return fmt.Errorf("unable to create related reference: %s", afterCreateErr.Error())
		// 	}
		// }

		if result.afterCreate != nil {
			afterCreateErr := result.afterCreate(&isolatedContext, &modelCopy)
			if afterCreateErr != nil {
				return fmt.Errorf("unable to perform pre object create hook: %s", afterCreateErr.Error())
			}
		}

		return nil
	})

	if createdErr != nil {
		respData = NewRespErr(500, HM{
			"msg": "unable to create new object",
			"err": createdErr.Error(),
		})
		return
	}

	return modelCopy, NewRespErr(200, HM{
		"created": true,
		"object":  ToDto(modelCopy, appctx, reqData).Unwrap(),
	})
}

func (result *CrudConfig[T, CtxType]) Generate() *CrudConfig[T, CtxType] {

	group := result.ParentGroup
	// model := result.Model
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
				ctx.AbortWithStatusJSON(403, HM{
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
				ctx.AbortWithStatusJSON(403, HM{
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
	if !result.disableEndpoints.Create {
		group.POST("", writePermissionMiddleware, func(ctx *gin.Context) {

			// create new object
			data, err := ctx.GetRawData()
			if err != nil {
				ctx.JSON(500, HM{
					"msg": "unable to get object data, when creating new one",
					"err": err.Error(),
				})
				return
			}

			// default fill from model tags
			parsedJson := gjson.ParseBytes(data)

			// req := result.RequestData(ctx)
			reqData := result.RequestData(ctx)

			_, result := result.CreateEntity(appctx, ctx, parsedJson, reqData)

			if result == nil {
				ctx.JSON(500, gin.H{
					"msg": "unexpected response",
				})
			} else {
				ctx.JSON(result.Httpcode, result.Data)
			}
		})
	}

	// get list
	if !result.disableEndpoints.List {
		group.GET("", func(ctx *gin.Context) {

			userAuthData := result.RequestData(ctx)

			listQueryParams := ListQueryParams{}
			ctx.BindQuery(&listQueryParams)

			// TODO put into crud group state, no need to get it each time manually
			modelDataStruct := result.TypeDataModel

			// filters
			// decode userdata from query
			filtersMap := HM{}

			if listQueryParams.PredefinedQuery != "" {
				var predefinedQErr *RespErr

				oldPage := listQueryParams.Page

				// overrides paging, sorting etc
				filtersMap, listQueryParams, predefinedQErr = result.ParsePredefinedQuery(listQueryParams)
				if predefinedQErr != nil {
					ctx.JSON(predefinedQErr.Httpcode, predefinedQErr.Data)
					return
				}

				listQueryParams.Page = oldPage

			} else {
				_filterValue := listQueryParams.Filter
				json.Unmarshal([]byte(_filterValue), &filtersMap)
			}

			filterCompiled := prepareFilterData[T, CtxType](filtersMap, result, modelDataStruct, userAuthData, listQueryParams)

			if !filterCompiled.IsOk() {
				ctx.JSON(200, HM{
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

			var joinClause string = ""
			var joinClauseArgs []any = []any{}
			joinClauseWhereCond := ""

			complexFiltersCount := len(filterData.ComplexFilters)
			if complexFiltersCount > 0 {
				userAuthData.log_format("processing %d complex filters ", complexFiltersCount)

				for _, it := range filterData.ComplexFilters {

					userAuthData.log_format("processing filter for %s", it.fiedName)

					func() {

						defer func() {
							rec := recover()
							if rec != nil {
								userAuthData.log_format("unable to process complex filter for field `%s`: %s", it.fiedName, rec)
							}
						}()

						val := it.inputValue

						if it.filterData.InputTransformer != nil {
							userAuthData.log_format(" filter has input transformer, applying...")
							val = it.filterData.InputTransformer(appctx, val)
						}

						typ := reflect.TypeOf(uint64(0))
						name := it.filterData.RelDestFieldName

						fakeApiTags := ApiTags{
							TableColumnName: fmt.Sprintf("%s.%s", it.filterData.RelTable, name),
							TypeKind:        reflect.Uint64,
							NativeType:      typ,
							Typ:             typ.Name(),
							Fillable:        true,
							FillName:        nil,
							Name:            &name,
						}

						processedComplexFieldSql, complexArgs, err := processFilterValueToSqlCond("", val, userAuthData, it.fiedName, fakeApiTags)
						if err != nil {
							userAuthData.log_format("unable to generate complex filter (%s) value :%s", it.fiedName, err.Error())
						} else {
							joinClauseArgs = append(joinClauseArgs, complexArgs)
							joinClauseWhereCond = processedComplexFieldSql

							joinedTblName := it.filterData.RelTable
							curTable := result.tableName

							joinClause = fmt.Sprintf("INNER JOIN %s ON %s = %s.%s", joinedTblName, it.RelFieldName(it.filterData.RelCurFieldName), curTable, result.objectIdField)
						}
					}()
				}
			}

			{
				dbQ := appctx.Db.Raw()

				finalSQLConds := filtersSql
				finalArgs := filterData.Args

				if joinClauseWhereCond != "" {
					finalSQLConds += " AND " + joinClauseWhereCond
					finalArgs = append(finalArgs, joinClauseArgs...)
				}

				// todo add field list for list request

				qB := dbQ.Where(finalSQLConds, finalArgs...)
				countQuery := appctx.Db.Raw().Table(result.tableName).Where(finalSQLConds, finalArgs...)

				if joinClause != "" {
					qB = qB.Joins(joinClause)
					countQuery = countQuery.Joins(joinClause)
				}

				// todo cache
				idField := fmt.Sprintf("%s.%s", result.tableName, result.primaryIdDbName)

				countQuery.Distinct(idField).Count(&totalItems)
				pagesCount := math.Ceil(float64(totalItems) / float64(filterData.PerPage))

				userAuthData.log_format("requst SQL: %s", finalSQLConds)

				sortOrder := "ASC"
				if listQueryParams.SortOrder == -1 {
					sortOrder = "DESC"
				}

				if filterData.Limit > 0 {
					qB = qB.Limit(filterData.Limit)
				}

				if filterData.Offset > 0 {
					qB = qB.Offset(filterData.Offset)
				}

				var sortFieldName string

				if listQueryParams.SortField != "" {
					sortFieldName = fmt.Sprintf("%s.%s", result.tableName, listQueryParams.SortField)

					sortOrderClause := fmt.Sprintf("%s %s", sortFieldName, sortOrder)
					qB = qB.Order(sortOrderClause)
				}

				itemIds := []map[string]any{}

				finalQuery := qB.Table(result.tableName)

				if sortFieldName != "" {
					finalQuery = finalQuery.Select(fmt.Sprintf("DISTINCT(%s)", idField), sortFieldName)
				} else {
					finalQuery = finalQuery.Distinct(idField)
				}

				respObject := finalQuery.Find(&itemIds)

				findErr := respObject.Error

				if findErr != nil {

					eId := uuid.NewString()

					log.Printf("db err : %s: %s", eId, findErr.Error())

					ctx.AbortWithStatusJSON(404, HM{
						"msg": "db err",
						"id":  eId,
					})
					return
				} else {

					var items []T

					// ids := []any{}

					idQueryFormat := fmt.Sprintf("%s = ?", result.objectIdField)

					for _, rowItem := range itemIds {

						idValue := rowItem[result.objectIdField]

						// ids = append(ids,)
						var item T

						errFindById := appctx.Db.Raw().First(&item, idQueryFormat, idValue).Error
						if errFindById != nil {
							userAuthData.log_format("unable to find item by id: %v %s", idValue, findErr.Error())
						} else {
							items = append(items, item)
						}
					}

					{

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
							ctx.JSON(200, HM{
								"items":       dtos,
								"pages":       pagesCount,
								"total_items": totalItems,
								"logs":        userAuthData.getDebugLogs(),
							})
						} else {
							ctx.JSON(200, HM{
								"items":       dtos,
								"pages":       pagesCount,
								"total_items": totalItems,
							})
						}
					}

					return
				}
			}
		})
	}

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
				filter[modelInfo.SoftDeleteField.TableColumnName] = 0
			} else {
				filtersMap := map[string]any{}
				_filterValue := ctx.Query("filter")
				json.Unmarshal([]byte(_filterValue), &filtersMap)

				// if admin request forcely wants to query `removed` data - no problem
				_, removeFilterExists := filtersMap[modelInfo.SoftDeleteField.FillName]
				if !removeFilterExists {
					// hide removed elements by default
					filtersMap[modelInfo.SoftDeleteField.FillName] = 0
				}
			}
		}

		// userFilterForced := false
		// userFilterKey := ""

		// todo move to compile time
		// eg generate Use(...) without ifs
		if modelInfo.UserReferenceField.Has && !reqData.IsAdmin {

			userId := reqData.AuthorizedUserId

			idResult := fmt.Sprintf("%v", userId)

			if userId == nil || idResult == "" {
				ctx.AbortWithStatusJSON(404, HM{
					"msg": "item not f0und",
				})
				return
			} else {

				// userFilterForced = true
				// userFilterKey = modelInfo.UserReferenceField.TableColumnName
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

			ctx.AbortWithStatusJSON(404, HM{
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

	if !result.disableEndpoints.Update {
		existingItems.PATCH("", writePermissionMiddleware, func(ctx *gin.Context) {

			var modelCopy T

			model, _ := ctx.Get("_eobj")
			modelCopy = model.(T)

			data, err := ctx.GetRawData()

			if err != nil {
				ctx.JSON(500, HM{
					"msg": "unable to get object data, when creating new one",
					"err": err.Error(),
				})
				return
			}
			parsed := gjson.ParseBytes(data)

			if !parsed.Exists() {
				ctx.JSON(500, HM{
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
				ctx.JSON(500, HM{
					"msg": "fill object fields erorr",
					"err": fillError.Error(),
				})
				return
			}

			saveError := appctx.DbTransaction(func(c AppContext[CtxType]) error {

				saveErr := c.Db.Save(ref)

				// todo remove
				if saveErr == nil {

					fieldsData := result.TypeDataModel

					req.log_format("saved succesfully")

					if fieldsData.UpdateExtraMethod {

						req.log_format("processing extra update method for entity")

						objUpdater, _ := any(ref).(OnUpdateEventHandler[CtxType, T])
						updateEventError := objUpdater.OnUpdate(&c, modelCopy, req)
						if updateEventError != nil {

							req.log_format("rollback update due to OnUpdate: %s", updateEventError.Error())
							return updateEventError
						}
					}
				} else {
					req.log_format("got an error while saving item")
				}

				return saveErr
			})

			if saveError != nil {

				repsJson := HM{
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

				_resultJson := HM{
					"item": resultItem,
				}

				if req.Debug {
					_resultJson["logs"] = debugLogs.lines
				}

				ctx.JSON(200, _resultJson)
			}

		})
	}

	if !result.disableEndpoints.Delete {
		existingItems.DELETE("", writePermissionMiddleware, func(ctx *gin.Context) {

			reqData := result.RequestData(ctx)

			responseData := map[string]any{}
			responseHttpCode := 200

			defer func() {

				if reqData.Debug {
					responseData["logs"] = reqData.getDebugLogs()
				}

				ctx.JSON(responseHttpCode, responseData)
			}()

			var modelCopy T

			model, _ := ctx.Get("_eobj")
			modelCopy = model.(T)

			delResp := result.DeleteEntity(appctx, modelCopy, reqData)

			responseData = delResp.Data
			responseHttpCode = delResp.Httpcode
		})
	}

	if !result.disableEndpoints.Get {
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

			ctx.JSON(200, HM{
				"item": ToDto(modelCopy, appctx, reqData).Unwrap(),
			})
		})
	}

	for _, relatedItem := range result.hasMultiple {

		cur := relatedItem

		existingItems.GET("/"+cur.PathSuffix, func(ctx *gin.Context) {

			isolated := &AppContext[CtxType]{
				Db:   appctx.Db,
				Data: result.App.Data,
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
