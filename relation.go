package main

type Relation[T any] interface {
	RelatedObjectFieldName() string
	SetUserId(id uint64)
	SetObjectId(obj *T)
}
