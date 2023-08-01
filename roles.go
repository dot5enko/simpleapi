package simpleapi

import (
	"log"

	"github.com/gin-gonic/gin"
)

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
