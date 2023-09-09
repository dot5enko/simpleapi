package simpleapi

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strings"

	"github.com/dot5enko/typed"
	"github.com/tidwall/gjson"
)

type complexFilter[CtxType any] struct {
	inputValue any
	fiedName   string
	filterData *HasManyConfig[CtxType]
}

func (c complexFilter[T]) RelFieldName(name string) string {
	return fmt.Sprintf("%s.%s", c.filterData.RelTable, name)
}

type filterData[CtxType any] struct {
	_filter          map[string]any
	QueryPlaceholder string
	Args             []any

	ComplexFilters []complexFilter[CtxType]

	Limit   int
	Offset  int
	PerPage int
}

// func (filterData) Compile() (string, []any) {

// }

var ErrNoAccess = fmt.Errorf("user doesn't have access")

type ListQueryParams struct {
	SortField string `form:"sort_field"`
	SortOrder int    `form:"order"`
	Page      int    `form:"page"`
	PerPage   int64  `form:"per_page"`
}

func processFilterValueToSqlCond(tableName string, filterValue any, userAuthData RequestData, filterFieldName string, fieldInfo ApiTags) (fQueryCond string, argProcessed any, err error) {
	mapVal, isMap := filterValue.(map[string]any)

	tableColumName := fieldInfo.TableColumnName

	if isMap {
		opName, ok := mapVal["op"].(string)

		if ok {
			filterGenerator, supported := supportedFilters[opName]

			if supported {

				var argVal any
				var errProcessingFilterVal error

				fname := tableColumName

				if tableName != "" {
					fname = fmt.Sprintf("%s.%s", tableName, tableColumName)
				}

				fQueryCond, argVal = filterGenerator(fname, mapVal)

				// convert back to gjson for simplicity of using force converting types methods
				valj, _ := json.Marshal(argVal)

				overridenField := fieldInfo

				needArray := opName == "in"
				if needArray {
					overridenField.NativeType = reflect.SliceOf(overridenField.NativeType)
				}

				argProcessed, errProcessingFilterVal = ProcessFieldType(overridenField, gjson.ParseBytes(valj), userAuthData)

				if errProcessingFilterVal == nil {
					return
				} else {
					err = errProcessingFilterVal
					return
				}
			} else {
				err = fmt.Errorf("filter %s for `%s` is not supported", opName, filterFieldName)
				return
			}
		}
	} else {

		// todo validate type
		// expose type processor same as supported filters

		fQueryCond = fmt.Sprintf("%s = ?", tableColumName)
		argProcessed = filterValue
		return
	}

	return
}

func prepareFilterData[T any, CtxType any](
	filtersMap map[string]any,
	crudConfig *CrudConfig[T, CtxType],
	modelDataStruct FieldsMapping,
	userAuthData RequestData,
	listQueryParams ListQueryParams,
) typed.Result[filterData[CtxType]] {

	complexFilters := []complexFilter[CtxType]{}

	filtersSqlWithPlaceholders := ""

	hasDisabledFields := len(crudConfig.disableFilterOverFields) > 0

	parts := []string{}
	filterArgs := []any{}

	// filter soft deleted item
	if modelDataStruct.SoftDeleteField.Has {
		if !userAuthData.IsAdmin { // always hide softly removed items from userland, no exceptions
			filtersMap[modelDataStruct.SoftDeleteField.FillName] = false
		} else {
			// if admin request forcely wants to query `removed` data - no problem
			_, removeFilterExists := filtersMap[modelDataStruct.SoftDeleteField.FillName]
			if !removeFilterExists {
				// hide removed elements by default
				filtersMap[modelDataStruct.SoftDeleteField.FillName] = false
			}

			userAuthData.log_format(" softremoved `%s` set to `%v`", modelDataStruct.SoftDeleteField.FillName, filtersMap[modelDataStruct.SoftDeleteField.FillName])

		}
	}

	// override user related fields to current auth user if its not an admin
	// todo make it type safe through generics
	// each table/entity should have it own type for id ?
	if modelDataStruct.UserReferenceField.Has && !userAuthData.IsAdmin {
		authId := userAuthData.AuthorizedUserId

		if authId == nil {
			return typed.ResultFailed[filterData[CtxType]](ErrNoAccess)
		} else {
			// now its working cause db_name == fill_name
			// todo fix to use fll name
			filtersMap[modelDataStruct.UserReferenceField.FillName] = userAuthData.AuthorizedUserId
		}

		userAuthData.log_format(" user reference field `%s` set to `%v`", modelDataStruct.UserReferenceField.FillName, filtersMap[modelDataStruct.UserReferenceField.FillName])
	}

	for filterFieldName, filterValue := range filtersMap {

		declaredFieldName, ok := modelDataStruct.ReverseFillFields[filterFieldName]

		if !ok {

			data, hasFilter := crudConfig.HasManyFilter(filterFieldName)

			if !hasFilter {
				userAuthData.log_format("field %s is not fillable, skipped", filterFieldName)
			} else {
				userAuthData.log_format("field %s is one to many filter, using filter data in next step", filterFieldName)

				complexFilters = append(complexFilters, complexFilter[CtxType]{
					filterData: data,
					inputValue: filterValue,
					fiedName:   filterFieldName,
				})
			}
			// field is not fillable
			continue
		} else {
			userAuthData.log_format("field %s is filterable", filterFieldName)
		}

		fieldInfo := modelDataStruct.Fields[declaredFieldName]

		// allow only whitelisted fields
		if !userAuthData.IsAdmin {
			_, canBeFiltered := modelDataStruct.Filterable[filterFieldName]

			if !canBeFiltered {
				continue
			}
		}

		// if result.CrudGroup.Config.DisableFilter
		if hasDisabledFields {
			_, disabled := crudConfig.disableFilterOverFields[filterFieldName]
			if disabled {

				userAuthData.log_format("filter by %s is disabled by conf", filterFieldName)

				continue
			}
		}

		sqlPart, sqlArg, filterProcessErr := processFilterValueToSqlCond(crudConfig.tableName, filterValue, userAuthData, filterFieldName, fieldInfo)

		if filterProcessErr != nil {
			userAuthData.log_format("unable to process filter %s value: %s ", filterFieldName, filterProcessErr.Error())
		} else {
			parts = append(parts, sqlPart)
			filterArgs = append(filterArgs, sqlArg)
		}
	}

	filtersSqlWithPlaceholders = strings.Join(parts, " AND ")

	userAuthData.log(func(logger *log.Logger) {
		logger.Print("filter SQL:")
		logger.Printf("`%s` + args %v", filtersSqlWithPlaceholders, filterArgs)
	})

	curPage := listQueryParams.Page
	if curPage <= 0 {
		curPage = 1
	}

	perPageVal := listQueryParams.PerPage
	if perPageVal <= 0 {
		perPageVal = int64(crudConfig.paging.PerPage)
	}

	limitVal := int(perPageVal)
	offsetVal := (curPage - 1) * int(perPageVal)

	// check sorting field
	if listQueryParams.SortField != "" {
		_, canBeSorted := modelDataStruct.Filterable[listQueryParams.SortField]

		if !canBeSorted {
			listQueryParams.SortField = ""
		}
	}

	return typed.ResultOk(filterData[CtxType]{
		QueryPlaceholder: filtersSqlWithPlaceholders,
		Args:             filterArgs,
		Limit:            limitVal,
		Offset:           offsetVal,
		PerPage:          int(perPageVal),
		ComplexFilters:   complexFilters,
		_filter:          filtersMap,
	})
}
