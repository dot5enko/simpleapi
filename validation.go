package simpleapi

import (
	"log"
	"reflect"
	"strings"
	"time"
)

type FieldValidation int

const (
	Unique FieldValidation = iota
	Email
	Required
)

type ValidationRuleSet struct {
	RequiredFields map[string]FieldValidation
}

type ApiTags struct {
	Validate *string
	Name     *string

	TypeKind reflect.Kind
	Typ      string

	Role *string

	Fillable bool
	Outable  bool
	Internal bool

	FillName *string
}

type FieldsMapping struct {
	TypeName string

	Fields map[string]ApiTags

	FillExtraMethod bool
	OutExtraMethod  bool

	Fillable []string
	Outable  []string
}

// source : https://stackoverflow.com/questions/56616196/how-to-convert-camel-case-string-to-snake-case
func ToSnake(camel string) (snake string) {
	var b strings.Builder
	diff := 'a' - 'A'
	l := len(camel)
	for i, v := range camel {
		// A is 65, a is 97
		if v >= 'a' {
			b.WriteRune(v)
			continue
		}
		// v is capital letter here
		// irregard first letter
		// add underscore if last letter is capital letter
		// add underscore when previous letter is lowercase
		// add underscore when next letter is lowercase
		if (i != 0 || i == l-1) && (          // head and tail
		(i > 0 && rune(camel[i-1]) >= 'a') || // pre
			(i < l-1 && rune(camel[i+1]) >= 'a')) { //next
			b.WriteRune('_')
		}
		b.WriteRune(v + diff)
	}
	return b.String()
}

func GetFieldTags[CtxType any](obj any) (objMapp FieldsMapping) {

	objMapp.Fields = make(map[string]ApiTags)
	objMapp.Outable = []string{}
	objMapp.Fillable = []string{}

	reflectedObject := reflect.ValueOf(obj)
	_type := reflect.Indirect(reflectedObject).Type()
	fields_count := _type.NumField()

	// check interfaces here once for app run
	{
		_, additionalFill := obj.(ApiDtoFillable[CtxType])
		objMapp.FillExtraMethod = additionalFill

		_, additionalDto := obj.(ApiDto[CtxType])
		objMapp.OutExtraMethod = additionalDto

		log.Printf(" -- type %s : extra out : %v, extra fill : %v", _type.Name(), additionalDto, additionalFill)
	}

	for i := 0; i < fields_count; i++ {

		result := ApiTags{
			Internal: false,
		}
		fieldData := _type.Field(i)

		declaredName := fieldData.Name
		defName := ToSnake(declaredName)

		ftype := fieldData.Type
		result.TypeKind = ftype.Kind()
		if result.TypeKind == reflect.Struct {
			result.Typ = ftype.PkgPath() + "/" + ftype.Name()
		}

		api, has_api := fieldData.Tag.Lookup("api")
		if has_api {
			if api == "-" {
				result.Internal = true
			}
		}

		fillable, has_fill := fieldData.Tag.Lookup("fill")
		if has_fill {
			if fillable == "-" {
				result.Fillable = false
			} else {
				result.Fillable = true
				result.FillName = &fillable
			}
		} else {
			result.Fillable = true
			result.FillName = &defName
		}

		// todo optimize
		tag, hasName := fieldData.Tag.Lookup("out")
		if hasName {

			if tag == "-" {
				result.Outable = false
			} else {
				result.Outable = true
				result.Name = &tag
			}
		} else {
			result.Outable = true
			result.Name = &defName
		}

		validate, hasValidate := fieldData.Tag.Lookup("validate")
		if hasValidate {
			result.Validate = &validate
		}

		role, hasRole := fieldData.Tag.Lookup("role")
		if hasRole {
			result.Role = &role
		}

		log.Printf(" --- field `%s`: tags : %#+v", fieldData.Name, tag)

		if !result.Internal && result.Fillable {
			objMapp.Fillable = append(objMapp.Fillable, declaredName)
		}

		if !result.Internal && result.Outable {
			objMapp.Outable = append(objMapp.Outable, declaredName)
		}

		objMapp.Fields[declaredName] = result
	}

	return objMapp
}

func GetObjectType(obj any) string {
	tobj := reflect.Indirect(reflect.ValueOf(obj)).Type()
	return tobj.PkgPath() + "." + tobj.Name()
}

func (m FieldsMapping) ToDto(obj any) map[string]any {

	result := map[string]any{}

	reflected := reflect.ValueOf(obj)

	for _, fieldName := range m.Outable {
		fieldInfo := m.Fields[fieldName]

		ivalue := reflected.FieldByName(fieldName).Interface()

		var val any = ivalue

		if fieldInfo.TypeKind == reflect.Struct {

			if fieldInfo.Typ == "time/Time" {

				switch ivalueTyped := ivalue.(type) {
				case time.Time:
					{
						val = ivalueTyped.Unix()
					}
				}

			}

		}

		result[*fieldInfo.Name] = val
	}

	return result
}

type FillFromDtoOptions struct {

	// not implemented
	DontAllowExtraFields bool
}
