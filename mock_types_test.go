package simpleapi

type MockAppContext struct {
}

type MockEvent struct {
	Id uint64

	SoftDeleted bool `api:"_deleted" simpleapi:"softdelete,adminonly"`
	Label       string
}
