package simpleapi

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"reflect"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (result *CrudConfig[T, CtxType]) GenerateListEndpoint(
	group *gin.RouterGroup,
	filterProcessor func(filter *filterData[CtxType]),
) {

	// model := result.Model
	appctx := result.App

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

		if filterProcessor != nil {
			filterProcessor(&filterData)
		}

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
