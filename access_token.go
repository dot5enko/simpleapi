package simpleapi

import (
	"fmt"
	"time"

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

func GetTokenOrCreate[CtxType any](appctx *AppContext[CtxType], user User, expiry time.Duration) (result Result[AccessToken]) {

	tNow := time.Now()
	var obj AccessToken

	findErr := appctx.Db.Where("user_id = ? && expired_at > ?", user.Id, tNow).First(&obj).Error

	if findErr != nil {
		// create new

		obj.CreatedAt = tNow
		obj.ExpiredAt = tNow.Add(expiry)
		obj.UserId = user.Id
		obj.Value = fmt.Sprintf("%d:%s:%d", tNow.Unix(), uuid.NewString(), user.Id)

		createError := appctx.Db.Create(&obj).Error
		if createError != nil {
			return ResultFailed[AccessToken](createError)
		} else {
			return ResultOk(obj)
		}

	} else {
		return ResultOk(obj)
	}
}

func UserByToken[T any](appCtx *AppContext[T], tok string) (result Result[User]) {

	var err error
	var resultToken AccessToken
	err = appCtx.Db.
		Find(&AccessToken{}).
		Where("value = ?", tok).
		First(&resultToken).
		Error

	if err != nil {
		result.SetFail(ErrTokenNotExist)
		return
	}

	if !resultToken.ExpiredAt.After(time.Now()) {
		result.SetFail(ErrTokenExpired)
		return
	}
	var resp User
	err = appCtx.Db.Where("id = ?", resultToken.UserId).First(&resp).Error

	if err != nil {
		result.SetFail(ErrUserNotExists)
		return
	}

	result.SetOk(resp)
	return
}
