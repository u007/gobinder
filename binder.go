package gobinder

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gobuffalo/uuid"
	graphql "github.com/graph-gophers/graphql-go"
	"github.com/u007/gobinder/lib"
	// "github.com/gobuffalo/validate/validators"
	"github.com/u007/lib/tools"
	"golang.org/x/net/context"

	// "strings"
	"fmt"
	"reflect"
)

var DEBUG_MUTATION bool = true
var SetCreatedUpdatedTimeOnSave bool = true

type ParamBinderOption struct {
	Permit []string
}

type BaseModel interface {
	Init()
}

type ModelBinder struct {
	model    interface{}
	Changes  map[string][]interface{} //[0: old, 1: new]
	ModelNew *bool

	Context context.Context
}

func (this *ModelBinder) SetsFromJSON(values map[string]interface{}, markChanged bool) error {
	val := reflect.Indirect(reflect.ValueOf(this.Model()))

	// logging(this.Context).Debugf("setsfromjson %#v", values)
	for i := range values {
		var fieldName string
		for c := 0; c < val.NumField(); c++ {
			// field := val.Field(c)
			fieldType := val.Type().Field(c)
			// logging(this.Context).Debugf("field: %s, tag: %#v", fieldType.Name, fieldType.Tag.Get("json"))
			// logging(this.Context).Debugf("checking Set %s=%#v", fieldType.Tag.Get("json"), i)
			if fieldType.Tag.Get("json") == i {
				fieldName = fieldType.Name
				break
			}
		}
		if fieldName == "" {
			logging(this.Context).Debugf("ignore missing %s", i)
			continue
		}
		// logging(this.Context).Debugf("Set %s=%#v", fieldName, values[i])
		if err := this.Set(fieldName, values[i], markChanged); err != nil {
			err2 := fmt.Errorf("Error setting %s=%+v, error: %+v", fieldName, values[i], err)
			return err2
		}
	}

	return nil
}

func (this *ModelBinder) Sets(values map[string]interface{}, markChanged bool) error {
	for i := range values {
		if err := this.Set(i, values[i], markChanged); err != nil {
			err2 := fmt.Errorf("Error setting %s=%+v, error: %+v", i, values[i], err)
			return err2
		}
	}

	return nil
}

func (this *ModelBinder) Model() interface{} {
	return this.model
}

func (this *ModelBinder) IsNew() bool {
	if this.ModelNew != nil {
		return *this.ModelNew
	}
	var isNew bool
	model := reflect.ValueOf(this.model).Elem()
	field := model.FieldByName("ID")

	if !field.IsValid() {
		panic(fmt.Sprintf("Cannot get filed id: %v:%s", model.Type().Name(), "ID"))
	}

	id := field.Interface().(uuid.UUID)
	if id == uuid.Nil {
		isNew = true
	} else {
		isNew = false
	}
	this.ModelNew = &isNew
	return *this.ModelNew
}

func (this *ModelBinder) ResetChange(name string) {
	model := reflect.ValueOf(this.model).Elem()
	field := model.FieldByName(name)

	if !field.IsValid() {
		panic(fmt.Sprintf("Cannot find field: %v:%s", model.Type().Name(), name))
	}
	//changed only reflect on those set by binderinit (and when not same as default) or binder.Set
	if _, ok := this.Changes[name]; ok {
		delete(this.Changes, name)
	}
}

func (this *ModelBinder) Changed(name string) bool {
	model := reflect.ValueOf(this.model).Elem()
	field := model.FieldByName(name)

	if !field.IsValid() {
		panic(fmt.Sprintf("Cannot find field: %v:%s", model.Type().Name(), name))
	}
	//changed only reflect on those set by binderinit (and when not same as default) or binder.Set
	if _, ok := this.Changes[name]; ok {
		return true
	}
	return false
}

func (this *ModelBinder) Dirty() bool {
	for _ = range this.Changes {
		return true
	}
	return false
}

func (this *ModelBinder) SetValue(name string, value reflect.Value, markChanged bool) error {
	model := reflect.ValueOf(this.model).Elem()
	field := model.FieldByName(name)

	if !field.CanSet() {
		return fmt.Errorf("Cannot set field: %v:%s", model.Type().Name(), name)
	}

	oriValue := this.Get(name)
	err := BindFieldValue(this.Context, name, &field, value)
	if err != nil {
		return fmt.Errorf("Cannot set field: %v %+v", model.Type().Name(), err)
	}

	if !markChanged {
		return nil
	}

	newValue := this.Get(name)
	newValueReflect := reflect.ValueOf(newValue)
	newValueIsNilPtr := reflect.ValueOf(newValue).Kind() == reflect.Ptr && reflect.ValueOf(newValue).IsNil()

	if newValueReflect.Kind() == reflect.Ptr {
		oldValue := oriValue
		if reflect.ValueOf(oriValue).Kind() == reflect.Ptr {

			if !reflect.ValueOf(oriValue).IsValid() || reflect.ValueOf(oriValue).IsNil() {
				if !newValueReflect.IsValid() || newValueReflect.IsNil() {
					//same nil
					return nil
				}

				//changed and new value isnt nil
				this.Changes[name] = []interface{}{oriValue, newValue}
				return nil
			}

			// if !reflect.ValueOf(oriValue).IsValid() || reflect.ValueOf(oriValue).IsNil() { //nil value
			// 	// var x *string
			// 	// field.Set(reflect.ValueOf(x))
			// 	oldValue = reflect.Zero(field.Type()).Interface()
			// } else {
			// 	oldValue = reflect.ValueOf(oriValue).Elem().Interface()
			// }

			if reflect.ValueOf(oriValue).IsValid() || !reflect.ValueOf(oriValue).IsNil() {
				if newValueIsNilPtr {
					this.Changes[name] = []interface{}{oriValue, reflect.Zero(newValueReflect.Type()).Interface()}
					return nil
				}
			}
		}
		if !newValueIsNilPtr && oldValue != reflect.ValueOf(newValue).Elem().Interface() {
			this.Changes[name] = []interface{}{oriValue, newValue}
		}
	} else {
		if eq, _ := lib.IsEqualValue(this.Context, oriValue, newValue); !eq {
			this.Changes[name] = []interface{}{oriValue, newValue}
		}
	}

	return nil
}

func (this *ModelBinder) ResetRelation() error {
	if err := ForEachField(this.Model(), func(i int, fieldVal *reflect.Value, fieldStruct reflect.StructField) error {
		// logging(this.Context).Debugf("field: %s=%#v", fieldStruct.Name, fieldVal)
		if !reflect.Indirect(*fieldVal).IsValid() {
			//already nil, ignore
			return nil
		}

		fieldType := reflect.Indirect(*fieldVal).Type()
		if fieldType.Kind() == reflect.Struct {
			newVal := reflect.Zero(fieldVal.Type())
			fieldVal.Set(newVal)
			// logging(this.Context).Debugf("Resetting struct: %s=%#v", fieldStruct.Name, newVal)
			this.ResetChange(fieldStruct.Name)
		} else if fieldType.Kind() == reflect.Slice {
			newVal := reflect.Zero(fieldVal.Type())
			fieldVal.Set(newVal)
			// logging(this.Context).Debugf("Resetting slice: %s=%#v", fieldStruct.Name, newVal)
			this.ResetChange(fieldStruct.Name)
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

// https://play.golang.org/p/PmRkzehLlfa - test field.kind() vs field.type()
func BindFieldValue(ctx context.Context, name string, pField *reflect.Value, value reflect.Value) error {
	// gcontext.Logger.Debugf("Field: %#v", *pField)
	if !pField.IsValid() {
		logging(ctx).Warnf("Isnt valid-field: %s, %v", name, pField)
		return fmt.Errorf("Field is not valid: %v:%v", name, pField)
	}

	if !pField.CanSet() {
		return fmt.Errorf("Field passed in is not settable %s: %#v", name, pField)
	}

	//handling assigning to nil of any types
	if pField.Kind() == reflect.Ptr && !value.IsValid() {
		zero := reflect.Zero(pField.Type())
		pField.Set(zero)
		return nil
	}

	var field reflect.Value
	var fieldType reflect.Type

	//example ptr https://play.golang.org/p/mFxm7nfGMss
	if pField.Kind() == reflect.Ptr {
		newField := reflect.New(pField.Type().Elem())
		field = newField.Elem()
		pField.Set(newField)
		fieldType = pField.Type().Elem()
	} else {
		field = *pField
		fieldType = field.Type()
	}

	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}

	if !value.IsValid() {
		value = reflect.New(fieldType).Elem()
	}

	valueKind := value.Kind().String()
	strValueType := value.Type().String()

	if fieldType == value.Type() {
		field.Set(value)
		return nil
	}

	if fieldType.String() == "uuid.UUID" && value.Type().String() == "string" {

		if value.Interface().(string) == "" {
			value = reflect.ValueOf(uuid.Nil)
			field.Set(value)
			return nil
		} else {
			uuidValue, err := uuid.FromString(value.Interface().(string))
			if err != nil {
				logging(ctx).Errorf("Unable to convert to uuid: %v", value.Interface())
			} else {
				value = reflect.ValueOf(uuidValue)
				field.Set(value)
				return nil
			}
		}
	}

	if fieldType.String() == "time.Time" && value.Type().String() == "string" {
		val := value.Interface().(string)
		if len(val) == 0 {
			value = reflect.ValueOf(time.Time{})
		} else {
			var thetime time.Time
			if timeI, err := strconv.ParseInt(val, 10, 64); err == nil {
				thetime = time.Unix(timeI, 0)
			} else {
				var err error
				// thetime, err = dateparse.ParseLocal(val)
				thetime, err = ParseISODateTime(val)
				// thetime, err := tools.TimeFromISOString(val)
				if err != nil {
					logging(ctx).Errorf("Unable to parse time: %s=%v %s", name, val, err.Error())
					return nil
				}
			}
			value = reflect.ValueOf(thetime)
		}
		field.Set(value)
		return nil
	}

	if fieldType.String() == "time.Time" && value.Type().String() == "graphql.Time" {
		val := value.Interface().(graphql.Time).Time
		value = reflect.ValueOf(val)
		field.Set(value)
		return nil
	}

	if fieldType.String() == "int" && value.Type().String() == "int32" {
		val := value.Interface().(int32)
		actualValue := int(val)
		value = reflect.ValueOf(actualValue)
		field.Set(value)
		return nil
	}

	if fieldType.String() == "float" && value.Type().String() == "float32" {
		val := value.Interface().(float32)
		actualValue := float64(val)
		value = reflect.ValueOf(actualValue)
		field.Set(value)
		return nil
	}

	switch strValueType {
	case "[]string":
		switch fieldType.String() {
		case "string":
			//convert to postgresql compatible
			// resValue = []string
			values := value.Interface().([]string)
			// for _, value := range values {
			// 	resValue = append(resValue, value)
			// }
			resString := "{" + strings.Join(values, ",") + "}"
			value = reflect.ValueOf(resString)
			field.Set(value)
			return nil
		}
	case "map[string]interface {}", "[]map[string]interface {}", "[]interface {}":
		switch fieldType.String() {
		case "string":
			//convert to postgresql compatible
			// resValue = []string
			// values := value.Interface().(map[string]interface{})
			// for _, value := range values {
			// 	resValue = append(resValue, value)
			// }
			b, err := json.Marshal(value.Interface())
			if err != nil {
				return err
			}
			value = reflect.ValueOf(string(b))
			field.Set(value)
			return nil
		}
	case "float64":
		switch fieldType.String() {
		case "int":
			//graphql always pass in integer as float64
			value = reflect.ValueOf(int(value.Interface().(float64)))
			field.Set(value)
			return nil
		case "int64":
			//graphql always pass in integer as float64
			value = reflect.ValueOf(int(value.Interface().(int64)))
			field.Set(value)
			return nil
		}
	case "string":
		switch fieldType.String() {
		case "int64":
			val, _ := tools.ParseInt64FromString(value.Interface().(string), 0)
			if fieldType.String() == "time.Time" {
				thetime := time.Unix(val, 0)
				value = reflect.ValueOf(thetime)
			} else {
				value = reflect.ValueOf(val)
			}
			field.Set(value)
			return nil
		case "int":
			val, _ := tools.ParseIntFromString(value.Interface().(string), 0)
			// logging(ctx).Debugf("%s assign to int %+v", name, val)
			if fieldType.String() == "time.Time" {
				thetime := time.Unix(int64(val), 0)
				value = reflect.ValueOf(thetime)
			} else {
				value = reflect.ValueOf(val)
			}

			field.Set(value)
			return nil
		case "time.Time":
			// logging(ctx).Debugf("field is time, value is string")
			val := value.Interface().(string)
			if len(val) == 0 {
				value = reflect.ValueOf(time.Time{})
			} else {
				var thetime time.Time
				if timeI, err := strconv.ParseInt(val, 10, 64); err == nil {
					thetime = time.Unix(timeI, 0)
				} else {
					var err error
					// thetime, err = dateparse.ParseLocal(val)
					// thetime, err := tools.TimeFromISOString(val)
					thetime, err = ParseISODateTime(val)
					if err != nil {
						logging(ctx).Errorf("Unable to parse time: %s=%v %s", name, val, err.Error())
						return nil
					}
				}
				value = reflect.ValueOf(thetime)
			}
			field.Set(value)
			return nil

		case "float64":
			val, _ := tools.ParseFloat64FromString(value.Interface().(string), 0.0)
			value = reflect.ValueOf(val)
			field.Set(value)
			return nil
		case "bool":
			val := value.Interface().(string)
			if val == "1" || strings.ToLower(val) == "true" {
				value = reflect.ValueOf(true)
				field.Set(value)
				return nil
			} else if val == "0" || strings.ToLower(val) == "false" {
				value = reflect.ValueOf(false)
				field.Set(value)
				return nil
			} else {
				logging(ctx).Errorf("Unable to convert to bool: %v", val)
			}
		} //switch
	case "int": // value field type
		switch fieldType.String() { //model field type
		case "int64":
			val := int64(value.Interface().(int))
			value = reflect.ValueOf(val)
			field.Set(value)
			return nil
		case "int32":
			val := int32(value.Interface().(int))
			value = reflect.ValueOf(val)
			field.Set(value)
			return nil
		}

	default:
		if fieldType.String() == strValueType {
			field.Set(value)
			return nil
		}
	} //end if string

	if valueKind == "slice" {
		destPointer := fieldType.Kind() == reflect.Ptr
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}

		//populate slices to slices
		if fieldType.Kind() == reflect.Slice {

			structType := fieldType.Elem()
			// var destSlice reflect.Value
			var destWriteSlice reflect.Value
			//create *[] or []
			// destWriteSlice = reflect.New(fieldType)
			destSlice := reflect.New(fieldType)
			destWriteSlice = destSlice.Elem()
			destWriteSlice.Set(reflect.MakeSlice(fieldType, 0, 0))

			//a slice class
			var singleSliceIsPointer bool
			for i := 0; i < value.Len(); i++ {
				row := value.Index(i)
				var structVal reflect.Value
				//is []*Class
				if structType.Kind() == reflect.Ptr {
					// structType = structType.Elem()
					structVal = reflect.New(structType.Elem()).Elem()
					singleSliceIsPointer = true
				} else {
					//is []Class
					structVal = reflect.New(structType).Elem()
				}

				values, ok := row.Interface().(map[string]interface{})
				if !ok {
					return fmt.Errorf("Binding value slice is not map[string]interface{}")
				}

				for k, v := range values {
					field, err := FieldByTagName(structVal.Interface(), "json", k)
					if err != nil {
						logging(ctx).Debugf("Missing field %v on %v", k, structVal.Type())
					} else {
						fieldValue := structVal.FieldByName(field.Name)

						if err := BindFieldValue(ctx, k, &fieldValue, reflect.ValueOf(v)); err != nil {
							return fmt.Errorf("Unable to bind %v on %v, value: %v, error: %+v",
								k, structVal.Type(), v, err)
						}
						// field.Set(v)
					}
				}

				// fmt.Println("loop %v=%v %#v", i, row, structVal)

				if singleSliceIsPointer {
					destWriteSlice.Set(reflect.Append(destWriteSlice, structVal.Addr()))
				} else {
					if destPointer {
						tmpValues := reflect.Append(destWriteSlice, structVal)
						destWriteSlice.Set(tmpValues)
					} else {
						destWriteSlice = reflect.Append(destWriteSlice, structVal)
						field.Set(destWriteSlice)
					}

				}
			} //each child rows

			if destPointer {
				field.Set(destSlice)
			} else {
				field.Set(destWriteSlice)
			}

			return nil

		} else if fieldType.Kind() == reflect.Struct {
			//saving []json (uid) from 1st record to struct
			structType := fieldType
			var structPointer reflect.Value
			var structVal reflect.Value
			//is *Class
			if structType.Kind() == reflect.Ptr {
				structPointer = reflect.New(structType.Elem())
			} else {
				//is Class
				structPointer = reflect.New(structType)
			}
			structVal = structPointer.Elem()

			if value.Len() == 0 {
				if destPointer {
					field.Set(structPointer)
				} else {
					field.Set(structVal)
				}
				return nil
			}

			row := value.Index(0)
			values, ok := row.Interface().(map[string]interface{})
			if !ok {
				return fmt.Errorf("Binding struct from slice is not map[string]interface{}")
			}

			for k, v := range values {
				field, err := FieldByTagName(structVal.Interface(), "json", k)
				if err != nil {
					logging(ctx).Debugf("Missing field %v on %v", k, structVal.Type())
				} else {
					fieldValue := structVal.FieldByName(field.Name)

					if err := BindFieldValue(ctx, k, &fieldValue, reflect.ValueOf(v)); err != nil {
						return fmt.Errorf("Unable to bind %v on %v, value: %v, error: %+v",
							k, structVal.Type(), v, err)
					}
					// field.Set(v)
				}
			}

			if destPointer {
				field.Set(structPointer)
			} else {
				field.Set(structVal)
			}

			return nil
			//struct
		} else {
			return fmt.Errorf("Destination %s is not slice", name)
		}
	}

	logging(ctx).Errorf("Data type mismatch field: %s, received: %v, expected: %v", name, strValueType, fieldType)
	return fmt.Errorf("Data type mismatch field: %s, received: %v, expected: %v", name, strValueType, fieldType)
}

func (this *ModelBinder) Set(name string, value interface{}, markChanged bool) error {
	rValue := reflect.ValueOf(value)

	return this.SetValue(name, rValue, markChanged)
}

func (this *ModelBinder) OldValue(name string) interface{} {
	values, has := this.Changes[name]
	if has {
		return values[0]
	}

	model := reflect.ValueOf(this.model).Elem()
	field := model.FieldByName(name)
	v := reflect.ValueOf(field)
	if field.Kind() == reflect.Ptr {
		return nil
	}

	z := reflect.Zero(v.Type())

	return z
}

func (this *ModelBinder) Get(name string) interface{} {
	model := reflect.ValueOf(this.model).Elem()
	field := model.FieldByName(name)

	if !field.IsValid() {
		panic(fmt.Sprintf("Cannot get field: %v:%s", model.Type().Name(), name))
	}
	return field.Interface()
}
