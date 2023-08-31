package simpleapi

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dot5enko/typed"
	"github.com/tidwall/gjson"
)

type filterData struct {
	_filter          map[string]any
	QueryPlaceholder string
	Args             []any

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

func prepareFilterData[T any, CtxType any](
	filtersMap map[string]any,
	crudConfig *CrudConfig[T, CtxType],
	modelDataStruct FieldsMapping,
	userAuthData RequestData,
	listQueryParams ListQueryParams,
) typed.Result[filterData] {

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
		}
	}

	// override user related fields to current auth user if its not an admin
	// todo make it type safe through generics
	// each table/entity should have it own type for id ?
	if modelDataStruct.UserReferenceField.Has && !userAuthData.IsAdmin {
		authId := userAuthData.AuthorizedUserId

		if authId == nil {
			return typed.ResultFailed[filterData](ErrNoAccess)
		} else {
			// now its working cause db_name == fill_name
			// todo fix to use fll name
			filtersMap[modelDataStruct.UserReferenceField.FillName] = userAuthData.AuthorizedUserId
		}
	}

	for filterFieldName, filterValue := range filtersMap {

		declaredFieldName, ok := modelDataStruct.ReverseFillFields[filterFieldName]
		fieldInfo := modelDataStruct.Fields[declaredFieldName]

		if !ok {

			// field is not fillable
			continue
		}

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
				continue
			}
		}

		mapVal, isMap := filterValue.(map[string]any)

		if isMap {

			opName, ok := mapVal["op"].(string)

			if ok {
				filterGenerator, supported := supportedFilters[opName]

				if supported {

					fQueryCond, argVal := filterGenerator(fieldInfo.TableColumnName, mapVal)

					// convert back to gjson for simplicity of using force converting types methods
					valj, _ := json.Marshal(argVal)

					argProcessed, errProcessingFilterVal := ProcessFieldType(fieldInfo, gjson.ParseBytes(valj), userAuthData)

					if errProcessingFilterVal == nil {
						parts = append(parts, fQueryCond)
						filterArgs = append(filterArgs, argProcessed)
					}
				}
			}
		} else {

			// todo validate type
			// expose type processor same as supported filters

			parts = append(parts, fmt.Sprintf("%s = ?", fieldInfo.TableColumnName))
			filterArgs = append(filterArgs, filterValue)
		}
	}

	filtersSqlWithPlaceholders = strings.Join(parts, " AND ")

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

	return typed.ResultOk(filterData{
		QueryPlaceholder: filtersSqlWithPlaceholders,
		Args:             filterArgs,
		Limit:            limitVal,
		Offset:           offsetVal,
		PerPage:          int(perPageVal),
		_filter:          filtersMap,
	})
}
