package simpleapi

import (
	"fmt"
	"reflect"
	"time"

	"github.com/tidwall/gjson"
)

// not in use
// as gorm doesn't support arbitrary types too :)
type DtoFieldTypeProcessor[T any] struct {
	Fill   func(jval gjson.Result) T
	Export func(fieldVal T) any
}

var fieldTypeProcessors map[string]DtoFieldTypeProcessor[any]

func RegisterFieldTypeProcessor(typeName string, processor DtoFieldTypeProcessor[any]) {
	fieldTypeProcessors[typeName] = processor
}

func ProcessFieldType(fieldInfo ApiTags, jsonFieldValue gjson.Result) (result any, err error) {
	defer func() {
		r := recover()

		if r != nil {
			err = fmt.Errorf("unable to fill from dto: %s. fields available", r)
			// br = true
		}
	}()

	fieldType := fieldInfo.NativeType
	fieldTypeKind := fieldType.Kind()

	// if field.IsZero() {
	// 	err = fmt.Errorf("unable to find a `%s` field on type : %s. field type and kind : %s %s type declared : %s",
	// 		fieldName, reflected.Type().Name(), fieldType.Name(), fieldTypeKind.String(), m.TypeName,
	// 	)
	// 	br = true
	// 	return
	// }

	var dtoData any

	switch fieldTypeKind {
	case reflect.Struct:

		if fieldInfo.Typ == "time/Time" {

			unixts := jsonFieldValue.Int()
			tread := time.Unix(unixts, 0)

			dtoData = tread

		} else {
			// todo optimize slow !!!

			processor, hasProcessor := fieldTypeProcessors[fieldInfo.Typ]

			if !hasProcessor {
				err = fmt.Errorf("unsupported field type to set from json: %s", fieldInfo.Typ)
				return
			} else {
				dtoData = processor.Fill(jsonFieldValue)
			}
		}
	case reflect.Int:
		intVal := jsonFieldValue.Int()
		dtoData = int(intVal)

	case reflect.Uint8:
		uintval := jsonFieldValue.Uint()

		if uintval > 255 {
			err = ErrNumberOverflow
			return
		} else {
			dtoData = uint8(uintval)
		}

	default:

		if fieldTypeKind >= 2 && fieldTypeKind <= 6 {
			// cast to int
			dtoData = jsonFieldValue.Int()
		} else {
			if fieldTypeKind >= 7 && fieldTypeKind <= 11 {
				dtoData = jsonFieldValue.Uint()
			} else {
				dtoData = jsonFieldValue.Value()
			}
		}
	}

	return dtoData, err

}
