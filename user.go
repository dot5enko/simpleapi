package simpleapi

import "time"

type User struct {
	Id   uint64 `gorm:"primaryKey"`
	Mail string `gorm:"index"`

	Password           string
	RecoveryKey        string `gorm:"index"`
	RecoveryKeyCreated *time.Time

	Confirmed        bool       `gorm:"confirmed"`
	ConfirmationSent *time.Time `gorm:"index"`
}

type UserToObject struct {
	Id uint64 `gorm:"primaryKey"`

	UserId   uint64 `gorm:"index"`
	ObjectId uint64 `gorm:"index"`
	Role     uint8
}
