package simpleapi

type UserToObject struct {
	Id uint64 `gorm:"primaryKey"`

	UserId   uint64 `gorm:"index"`
	ObjectId uint64 `gorm:"index"`
	Role     uint8
}

type UserFeatures interface {
	GetId() uint64
}
