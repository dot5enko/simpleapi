package simpleapi

import (
	"fmt"
	"log"
	"reflect"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

func RelatedItemHandlerImpl[OfType any, CtxType any, RelatedType any](appctx *AppContext[CtxType], idGetter RelatedObjectIdGetter[OfType], parentIdField, apiPath string, idFieldName string, req RequestData) {

	ctx := appctx.Request

	idParam := ctx.Param("id")

	model := *new(OfType)
	modelCopy := model

	_db := appctx.Db.Raw()

	q := fmt.Sprintf("%s = ?", idFieldName)

	// todo validate
	errFirst := _db.Model(&model).Where(q, idParam).First(&modelCopy).Error

	if errFirst != nil {
		ctx.JSON(404, gin.H{
			"msg": "related object not found",
			"err": errFirst.Error(),
		})
	} else {

		func() {

			defer func() {

				rec := recover()
				if rec != nil {

					ctx.JSON(500, gin.H{
						"msg":     "recovered",
						"recover": rec,
						"stack":   string(debug.Stack()),
					})
					return
				}
			}()

			relatedObjectId := idGetter(&modelCopy)

			outItems := []RelatedType{}

			// // dynamically create pointer to array of items
			// relatedModelType := reflect.SliceOf(reflect.ValueOf(relatedItems.RelatedModel).Type())
			// _outItems := reflect.MakeSlice(relatedModelType, 0, 10)
			// outPtr := reflect.New(relatedModelType)
			// outPtr.Elem().Set(_outItems)

			// outItems := outPtr.Interface()

			whereCond := fmt.Sprintf("%s = ?", parentIdField)

			// todo cache
			relatedModel := *new(RelatedType)

			result := _db.Model(relatedModel).Where(whereCond, relatedObjectId).Find(&outItems)

			err := result.Error

			if err != nil {
				ctx.JSON(500, gin.H{
					"msg": "unable to get related items",
					"err": err.Error(),
				})
				return
			} else {

				// todo cache
				morphed := []any{}

				// arraRef := outPtr.Elem()
				// arrLen := arraRef.Len()

				// for i := 0; i < arrLen; i++ {
				// 	it := arraRef.Index(i).Interface()
				// morphed = append(morphed, toDto[any](it, appctx, 0).Unwrap())
				// }

				for _, it := range outItems {
					morphed = append(morphed, ToDto(it, appctx, req).Unwrap())
				}

				ctx.JSON(200, gin.H{
					"items": morphed,
				})
			}
		}()
	}

}

func RelatedModels[Related any, CtxType any, OfType any](
	groupConfig *CrudGroup[CtxType],
	pathSuffix, childIdField string,
	idgetter RelatedObjectIdGetter[OfType],
) ApiObjectRelation[OfType, CtxType] {

	return ApiObjectRelation[OfType, CtxType]{
		ParentObjectIdField: childIdField,
		PathSuffix:          pathSuffix,
		ItemHandler: func(
			appctx *AppContext[CtxType],
			that *ApiObjectRelation[OfType, CtxType],
			req RequestData,
		) {
			RelatedItemHandlerImpl[OfType, CtxType, Related](
				appctx,
				idgetter,
				that.ParentObjectIdField,
				that.PathSuffix,
				groupConfig.Config.ObjectIdFieldName,
				req,
			)
		},
	}
}

func createRelAfterSave[T any, CtxType any](appctx *AppContext[CtxType], obj *T, relTable string) error {

	var relation UserToObject

	// todo optimize
	val := reflect.Indirect(reflect.ValueOf(obj))

	relation.ObjectId = val.FieldByName("Id").Uint()
	relation.UserId = GetUserId(appctx.Request)

	return appctx.Db.Raw().Table(relTable).Create(&relation).Error
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
