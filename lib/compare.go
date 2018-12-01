package lib

import (
	"context"
	"reflect"
)

func IsEqualValue(ctx context.Context, valueA interface{}, valueB interface{}) (bool, error) {
	typeA := reflect.TypeOf(valueA)

	//slice compare
	if typeA.Kind() == reflect.Slice {
		valA := reflect.ValueOf(valueA)
		valB := reflect.ValueOf(valueB)

		if valA.Len() != valB.Len() {
			return false, nil
		}

		for i := 0; i < valA.Len(); i++ {
			indexValA := reflect.Indirect(valA.Index(i))
			indexValB := reflect.Indirect(valB.Index(i))

			if indexValA != indexValB {
				return false, nil
			}
		}

		return true, nil
	}

	//struct compare
	if typeA.Kind() == reflect.Struct && typeA.String() != "time.Time" {
		valA := reflect.ValueOf(valueA)
		valB := reflect.ValueOf(valueB)

		if valA.NumField() != valB.NumField() {
			return false, nil
		}

		for i := 0; i < valA.NumField(); i++ {
			fieldA := valA.Field(i)
			nameA := valA.Type().Field(i).Name

			fieldB := valB.FieldByName(nameA)
			if !fieldB.IsValid() {
				logging(ctx).Debugf("field: %s missing in second value", nameA)
				return false, nil
			}

			indexValA := reflect.Indirect(fieldA).Interface()
			indexValB := reflect.Indirect(fieldB).Interface()

			if reflect.ValueOf(indexValA).Kind() == reflect.Map || (reflect.ValueOf(indexValA).Kind() == reflect.Struct && typeA.String() != "time.Time") {
				if is, err := IsEqualValue(ctx, indexValA, indexValB); err != nil {
					return is, err
				} else if !is {
					return false, nil
				}

				continue
			}

			if reflect.ValueOf(indexValA).Kind() == reflect.Slice {
				if is, err := IsEqualValue(ctx, indexValA, indexValB); err != nil {
					return is, err
				} else if !is {
					return false, nil
				}

				continue
			}

			if indexValA != indexValB {
				return false, nil
			}
		}

		return true, nil
	}

	return valueA == valueB, nil
}
