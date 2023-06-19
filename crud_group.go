package simpleapi

type CrudGroup[T any] struct {
	Ctx    AppContext[T]
	Config CrudGroupConfig
}

type CrudGroupConfig struct {
	Auth              bool
	ObjectIdFieldName string
}

func NewCrudGroup[T any](ctx AppContext[T], config CrudGroupConfig) *CrudGroup[T] {

	result := &CrudGroup[T]{
		Ctx:    ctx,
		Config: config,
	}

	return result

}
