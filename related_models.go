package main

import (
	"fmt"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

func RelatedItemHandlerImpl[OfType any, CtxType any, RelatedType any](appctx *AppContext[CtxType], idGetter RelatedObjectIdGetter[OfType], parentIdField, apiPath string) {

	ctx := appctx.Request

	idParam := ctx.Param("id")

	model := *new(OfType)
	modelCopy := model

	// todo validate
	errFirst := appctx.Db.Model(&model).Where("id = ?", idParam).First(&modelCopy).Error

	if errFirst != nil {
		ctx.JSON(404, gin.H{
			"msg": "object not found",
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

			tx := appctx.Db

			// todo cache
			relatedModel := *new(RelatedType)

			result := tx.Model(relatedModel).Where(whereCond, relatedObjectId).Find(&outItems)

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
					morphed = append(morphed, toDto(it, appctx, 0).Unwrap())
				}

				ctx.JSON(200, gin.H{
					"items": morphed,
				})
			}
		}()
	}

}

func RelatedModels[Related any, CtxType any, OfType any](pathSuffix, childIdField string, idgetter RelatedObjectIdGetter[OfType]) ApiObjectRelation[OfType, CtxType] {
	return ApiObjectRelation[OfType, CtxType]{
		ParentObjectIdField: childIdField,
		PathSuffix:          pathSuffix,
		ItemHandler: func(appctx *AppContext[CtxType], that *ApiObjectRelation[OfType, CtxType]) {
			RelatedItemHandlerImpl[OfType, CtxType, Related](appctx, idgetter, that.ParentObjectIdField, that.PathSuffix)
		},
	}
}
