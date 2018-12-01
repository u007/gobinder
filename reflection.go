package gobinder

import (
	"fmt"
	"reflect"
)

func IsStructOrIsSlicesOfStruct(value interface{}) bool {
	valType := reflect.TypeOf(value)
	if valType == nil {
		return true //is interface{}
	}
	// fmt.Printf("valtype: %#v | %#v\n", valType)
	if valType.Kind() == reflect.Ptr {
		valType = valType.Elem()
	}

	// fmt.Printf("kind? %#v, type: %#v\n", valType.Kind().String(), valType.String())
	if valType.Kind() == reflect.Struct && valType.String() != "time.Time" {
		return true
	}

	if valType.Kind() != reflect.Slice {
		return false
	}

	valType = valType.Elem() // a single slice type
	if valType.Kind() == reflect.Ptr {
		valType = valType.Elem()
	}

	if valType.Kind() == reflect.Struct && valType.String() != "time.Time" {
		return true
	}

	return false
}

// @return true if parameter is []struct, and not []time.Time
func IsSlicesOfStruct(slices interface{}) bool {
	valType := reflect.TypeOf(slices)
	if valType.Kind() == reflect.Ptr {
		valType = valType.Elem()
	}

	if valType.Kind() != reflect.Slice {
		return false
	}

	valType = valType.Elem() // a single slice type
	if valType.Kind() == reflect.Ptr {
		valType = valType.Elem()
	}

	if valType.Kind() == reflect.Struct && valType.String() != "time.Time" {
		return true
	}

	return false
}

type ForEachBlock func(int, *reflect.Value, reflect.StructField) error

func ForEachField(st interface{}, block ForEachBlock) error {
	val := reflect.Indirect(reflect.ValueOf(st))
	if val.Kind() != reflect.Struct {
		return fmt.Errorf("value is not struct: %s", val.Kind())
	}

	for i := 0; i < val.NumField(); i++ {
		fieldStruct := val.Type().Field(i)
		field := val.Field(i)
		fieldType := reflect.TypeOf(st)
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}
		if err := block(i, &field, fieldStruct); err != nil {
			return err
		}
	}
	return nil
}

func FieldByTagName(value interface{}, tagName string, tagValue string) (reflect.StructField, error) {
	structType := reflect.TypeOf(value)
	return FieldByTagNameViaType(structType, tagName, tagValue)
}

func FieldByTagNameViaType(structType reflect.Type, tagName string, tagValue string) (reflect.StructField, error) {
	if structType.Kind() == reflect.Ptr {
		structType = structType.Elem()
	}
	if structType.Kind() == reflect.Slice {
		structType = structType.Elem()
	}

	// fmt.Printf("fieldbytagname: %#v (%s)=%s - %#v\n", structType, tagName, tagValue)
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		if field.Tag.Get(tagName) == tagValue {
			return field, nil
		}
		// fmt.Printf("fieldbytagname: %#v - %v vs %v\n", structType.Name(), field.Tag.Get(tagName), tagValue)
	}
	return reflect.StructField{}, fmt.Errorf("Tag [%s] field missing %v", tagName, tagValue)
}
