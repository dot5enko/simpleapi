package simpleapi

import (
	"fmt"
	"log"
	"math"
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

var fieldTypeProcessors = map[string]DtoFieldTypeProcessor[any]{}

func RegisterFieldTypeProcessor[T any](typeName string, processor DtoFieldTypeProcessor[T]) {

	converted, _ := any(processor).(DtoFieldTypeProcessor[any])
	fieldTypeProcessors[typeName] = converted
}

func ProcessFieldType(fieldInfo ApiTags, jsonFieldValue gjson.Result, req RequestData) (result any, err error) {
	defer func() {
		r := recover()

		if r != nil {
			err = fmt.Errorf("unable to fill from dto: %s. fields available", r)
			// br = true

			req.log(func(logger *log.Logger) {
				logger.Printf("unable to fill from dto: %s. fields available", r)
			})
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

	req.log(func(logger *log.Logger) {
		logger.Printf(" [%s] field detected typ : %s", *fieldInfo.Name, fieldTypeKind.String())
	})

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
		intval := jsonFieldValue.Int()

		if intval > math.MaxInt || intval < math.MinInt {
			err = ErrNumberOverflow
			return
		} else {
			dtoData = int(intval)
		}
	case reflect.Int8:
		intval := jsonFieldValue.Int()

		if intval > math.MaxInt8 || intval < math.MinInt8 {
			err = ErrNumberOverflow
			return
		} else {
			dtoData = int8(intval)
		}

	case reflect.Int16:
		intval := jsonFieldValue.Int()

		if intval > math.MaxInt16 || intval < math.MinInt16 {
			err = ErrNumberOverflow
			return
		} else {
			dtoData = int16(intval)
		}

	case reflect.Int32:
		intval := jsonFieldValue.Int()

		if intval > math.MaxInt32 || intval < math.MinInt32 {
			err = ErrNumberOverflow
			return
		} else {
			dtoData = int32(intval)
		}

	case reflect.Uint8:
		uintval := jsonFieldValue.Uint()

		if uintval > math.MaxUint8 {
			err = ErrNumberOverflow
			return
		} else {
			dtoData = uint8(uintval)
		}
	case reflect.Uint16:
		uintval := jsonFieldValue.Uint()

		if uintval > math.MaxUint16 {
			err = ErrNumberOverflow
			return
		} else {
			dtoData = uint16(uintval)
		}

	case reflect.Uint32:
		uintval := jsonFieldValue.Uint()

		if uintval > math.MaxUint32 {
			err = ErrNumberOverflow
			return
		} else {
			dtoData = uint32(uintval)
		}

	case reflect.Uint64:
		dtoData = jsonFieldValue.Uint()
	case reflect.Int64:
		intval := jsonFieldValue.Int()
		dtoData = int64(intval)
	case reflect.String:
		dtoData = jsonFieldValue.String()
	case reflect.Uint:
		uintval := jsonFieldValue.Uint()

		if uintval > math.MaxUint {
			err = ErrNumberOverflow
			return
		} else {
			dtoData = uint(uintval)
		}
	case reflect.Bool:
		boolval := jsonFieldValue.Bool()

		// todo validate ?

		dtoData = boolval
	default:

		req.log(func(logger *log.Logger) {
			logger.Printf(" [%s] field defaulted while converting from input data (json.Value), typ: %s", *fieldInfo.Name, fieldInfo.Typ)
		})

		processor, hasProcessor := fieldTypeProcessors[fieldInfo.Typ]

		if !hasProcessor {

			req.log(func(logger *log.Logger) {
				logger.Printf(" -> not found a specific processor, using .Value()")
			})

			dtoData = jsonFieldValue.Value()
		} else {
			dtoData = processor.Fill(jsonFieldValue)
		}

	}

	return dtoData, err

}
