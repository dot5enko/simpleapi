package simpleapi

type FieldValidation int

const (
	Unique FieldValidation = iota
	NotEmpty
	Email
	Required
)
