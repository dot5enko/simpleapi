package simpleapi

import (
	"fmt"
	"time"

	typed "github.com/cldfn/utils"
	"github.com/google/uuid"
)

type AccessToken struct {
	Id     uint64 `gorm:"primaryKey"`
	UserId uint64 `gorm:"index"`

	CreatedAt time.Time
	ExpiredAt time.Time `gorm:"index"`

	Value string `gorm:"index"`
}

var (
	ErrTokenExpired  error = fmt.Errorf("token expired")
	ErrTokenNotExist       = fmt.Errorf("token not exist")
	ErrUserNotExists       = fmt.Errorf("user not exists, this shoult not happen")
)

func GetTokenOrCreate(appctx *AppContext, user UserFeatures, expiry time.Duration) (result typed.Result[AccessToken]) {

	tNow := time.Now()
	var obj AccessToken

	findResult := FindFirstWhere[AccessToken](appctx.Db, "user_id = ? and expired_at > ?", user.GetId(), tNow)

	findErr := findResult.UnwrapError()

	if findErr != nil {
		// create new

		obj.CreatedAt = tNow
		obj.ExpiredAt = tNow.Add(expiry)
		obj.UserId = user.GetId()
		obj.Value = fmt.Sprintf("%d:%s:%d", tNow.Unix(), uuid.NewString(), user.GetId())

		createError := appctx.Db.Create(&obj)
		if createError != nil {
			return typed.ResultFailed[AccessToken](createError)
		} else {
			return typed.ResultOk(obj)
		}

	} else {
		return findResult
	}
}

func UserByToken[UserType any, T any](appCtx *AppContext, tok string) (result typed.Result[UserType]) {

	var err error
	var resultToken AccessToken

	findResult := FindFirstWhere[AccessToken](appCtx.Db, "value = ?", tok)

	err = findResult.UnwrapError()

	if err != nil {
		result.SetFail(ErrTokenNotExist)
		return
	}

	resultToken = findResult.Unwrap()

	if !resultToken.ExpiredAt.After(time.Now()) {
		result.SetFail(ErrTokenExpired)
		return
	}

	return FindFirstWhere[UserType](appCtx.Db, "id = ?", resultToken.UserId)
}
