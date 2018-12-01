package dgraph

import (
	"fmt"
	"reflect"
	"strings"

	_ "github.com/u007/gobinder"

	"github.com/dgraph-io/dgo/protos/api"
)

// update relation from arguments, but ensure to have saved this record first
// args.predicate_id = single string
// args.predicate = [{id: '0x02'}, {id:'0x03'}] - save multiple
// args.predicate_id = "" to delete existing relation
// args.predicate = [] - to delete all existing relation
// TODO check for update single relation to see if its same as before, if yes, ignore updatedFields
func (this *ModelBinder) UpdateRelationFromGraphQLArgs(tx *DGraphTxn, values interface{}, permitted []string, markChanged bool) error {
	ctx := this.Context
	model := this.Model()
	modelVal := reflect.Indirect(reflect.ValueOf(model))
	val := reflect.Indirect(reflect.ValueOf(values))

	idField := modelVal.FieldByName("UID")
	id := idField.Interface().(string)
	if id == "" {
		return fmt.Errorf("Invalid id")
	}

	updatedFields := []string{}
	for c := 0; c < val.NumField(); c++ {
		name := val.Type().Field(c).Name
		if val.Field(c).Kind() == reflect.Ptr && val.Field(c).IsNil() {
			// logging(this.Context).Debugf("ignore empty %s", name)
			continue
		}
		valueField := reflect.Indirect(val.Field(c))

		foundField := false
		dbName := strings.ToLower(string(name[0])) + name[1:]
		nestedRelation := false
		if dbName == "iD" {
			dbName = "uid"
		}

		if strings.HasSuffix(dbName, "_id") {
			dbName = dbName[0 : len(dbName)-3]
		}

		save := false
		for _, field := range permitted {
			// logging(ctx).Debugf("permitted: %s, dbname: %s", field, dbName)
			if strings.HasSuffix(field, ":nested") {
				nestedRelation = true
				field = field[0 : len(field)-7]
			}
			if dbName == field || field == "*" {
				save = true
				break
			}
		}
		if !save {
			continue //skipping
		}

		logging(ctx).Debugf("UpdateRelationFromGraphQLArgs %s of model: %s : %s", dbName, reflect.TypeOf(this.Model()), id)
		for d := 0; d < modelVal.NumField(); d++ {
			fieldName := modelVal.Type().Field(d).Name
			fieldType := modelVal.Type().Field(d).Type
			fieldJsonName := modelVal.Type().Field(d).Tag.Get("json")

			// logging(ctx).Debugf("comparing: %s vs %s", fieldJsonName, dbName)
			if fieldJsonName != dbName {
				continue
			}

			if fieldType.Kind() == reflect.Ptr {
				fieldType = fieldType.Elem()
			}

			foundField = true
			logging(ctx).Debugf("Found %s of %s, kind: %s", dbName, id, fieldType.Kind())
			if fieldType.Kind() == reflect.Struct {
				// logging(ctx).Debugf("is struct: %s", fieldJsonName)
				if fieldType.Name() == "Time" {
					continue //ignoring time struct
				}

				logging(ctx).Debugf("single relation type: %s", valueField.Type().Name())
				//single update

				if !nestedRelation {
					//expected ID as string
					if valueField.Type().Name() != "string" {
						return fmt.Errorf("Expecting %s as string", name)
					}
				}
				// logging(ctx).Debugf("found struct: %s: %#v=%#v", name, fieldType.Name(), valueField.Interface())
				//TODO check update single struct

			} else if fieldType.Kind() == reflect.Slice {
				// logging(ctx).Debugf("is slice: %s", fieldJsonName)
				if valueField.Type().Kind() != reflect.Slice {
					return fmt.Errorf("Expecting %s as slice with ID", name)
				}

				// checks for id field
				if !nestedRelation {
					for vI := 0; vI < valueField.Len(); vI++ {
						valueRow := valueField.Index(vI)
						rowID := valueRow.FieldByName("ID")
						if !rowID.IsValid() {
							logging(ctx).Debugf("missing id field")
							return fmt.Errorf("Expecting index %d  to contain ID", vI)
						}

						if rowID.Type().Name() != "string" {
							logging(ctx).Debugf("id is not string")
							return fmt.Errorf("Expecting index %d  to contain ID as string type", vI)
						}
					}
				}

			} else {
				continue // skip none struct nor slice
			}

			logging(ctx).Debugf("Deleting existing %s of %s", dbName, id)
			if nestedRelation {
				//delete actual record, to ensure no hanging relation
				//query existing relation for id, and delete all object
				if err := DeleteRelationField(tx, ctx, model, fieldName, false); err != nil {
					return err
				}
			} //is nestedRelation
			//dgraph direct delete existing relationship
			if _, err := tx.MutateDeleteField(ctx, id, dbName, nil, false); err != nil {
				logging(ctx).Errorf("unable to delete field: %+v", err)
				return err
			}

			sets := []*api.NQuad{}
			if fieldType.Kind() == reflect.Struct {
				updatedFields = append(updatedFields, fieldName)
				if !nestedRelation {
					//saving model_id relation
					relationID, ok := valueField.Interface().(string)
					if !ok {
						return fmt.Errorf("Field %s is not ID string", dbName)
					}
					if relationID == "" {
						continue // no data for _id
					}

					sets = append(sets, &api.NQuad{Subject: id, Predicate: dbName, ObjectId: relationID})

				} else {
					//saving single nested struct
					deleteField := reflect.Indirect(valueField.FieldByName("DELETE_"))
					if deleteField.IsValid() && deleteField.Interface().(bool) == true {
						logging(ctx).Debugf("Deleting %s struct, not inserting child", name)
						break
					}

					newChild := reflect.New(fieldType)
					childBinder := NewModelBinder(tx, ctx, newChild.Interface())
					childPermitted := []string{"*"}
					childBinder.SetsFromGraphQLArgs(valueField.Interface(), childPermitted, true)
					logging(ctx).Debugf("Saving Child %s %#v", name, newChild.Elem().Interface())

					verrs, err := SaveBinder(tx, ctx, &childBinder)
					if verrs != nil && verrs.HasAny() {
						return fmt.Errorf("Child %s Validation errors ", name, verrs.Errors)
					} else if err != nil {
						return fmt.Errorf("Child %s, error: %+v", name, err)
					}

					childID := newChild.Elem().FieldByName("UID")
					// logging(ctx).Debugf("child: %#v, id: %#v", newChild.Elem(), rowID)
					logging(ctx).Debugf("relation: %s, %s set: %v", id, dbName, childID.Interface())
					sets = append(sets, &api.NQuad{Subject: id, Predicate: dbName, ObjectId: childID.Interface().(string)})
				}
			} // is struct

			if fieldType.Kind() == reflect.Slice {
				updatedFields = append(updatedFields, fieldName)
				for vI := 0; vI < valueField.Len(); vI++ {
					valueRow := valueField.Index(vI)
					// logging(ctx).Debugf("child: %s, row: %#v", dbName, valueRow.Interface())
					rowID := valueRow.FieldByName("ID")

					if rowID.IsValid() && rowID.Interface().(string) != "" {
						//relates by id
						logging(ctx).Debugf("relation: %s, %s append: %v", id, dbName, rowID.Interface())
						sets = append(sets, &api.NQuad{Subject: id, Predicate: dbName, ObjectId: rowID.Interface().(string)})
					} else {
						//is empty, create nested child
						childType := fieldType.Elem()
						if childType.Kind() == reflect.Ptr {
							childType = childType.Elem()
						}
						// logging(ctx).Debugf("creating new child %s (%s)", dbName, fieldType.Kind())
						newChild := reflect.New(childType)
						childBinder := NewModelBinder(tx, ctx, newChild.Interface())
						childPermitted := []string{"*"}
						childBinder.SetsFromGraphQLArgs(valueRow.Interface(), childPermitted, true)
						logging(ctx).Debugf("Saving Child %s %#v", name, newChild.Elem().Interface())

						verrs, err := SaveBinder(tx, ctx, &childBinder)
						if verrs != nil && verrs.HasAny() {
							return fmt.Errorf("Child %s Validation errors ", name, verrs.Errors)
						} else if err != nil {
							return fmt.Errorf("Child %s, error: %+v", name, err)
						}

						rowID = newChild.Elem().FieldByName("UID")
						// logging(ctx).Debugf("child: %#v, id: %#v", newChild.Elem(), rowID)
						logging(ctx).Debugf("relation: %s, %s append: %v", id, dbName, rowID.Interface())
						sets = append(sets, &api.NQuad{Subject: id, Predicate: dbName, ObjectId: rowID.Interface().(string)})
					} // is nested child value

				} //each slice of value
			} //each slice

			if len(sets) > 0 {
				if _, err := tx.MutateSet(ctx, sets, false); err != nil {
					return err
				}

				//TODO auto fetch

			}

		} //each model field
		if !foundField {
			logging(this.Context).Debugf("ignore missing relation: %s", dbName)
		}
		// modelField := modelVal.FieldByName(dbName)
	} //each values field

	if len(updatedFields) > 0 {
		hook, ok2 := model.(BaseAfterSaveableRelation)
		if ok2 {
			if err := hook.AfterSaveRelation(tx, ctx, this, updatedFields...); err != nil {
				// gcontext.Logger.Errorf(fmt.Sprintf("postsave error: %+v", err))
				return err //return original error
			}
		}
	}

	return nil
}

// @param values - json value to populate to model
// @param permitted - list of field names to allow for populate
// @param markChanged - to allow binder to indicate this field is dirty (when set from input form, true, otherwise if set from dgraph databases: false)
func (this *ModelBinder) SetsFromGraphQLArgs(values interface{}, permitted []string, markChanged bool) error {
	modelVal := reflect.Indirect(reflect.ValueOf(this.Model()))
	val := reflect.Indirect(reflect.ValueOf(values))

	for c := 0; c < val.NumField(); c++ {
		field := val.Field(c)
		name := val.Type().Field(c).Name
		if field.Kind() == reflect.Ptr && field.IsNil() {
			// logging(this.Context).Debugf("ignore empty %s", name)
			continue
		}
		foundField := false
		dbName := strings.ToLower(string(name[0])) + name[1:]
		if dbName == "iD" {
			dbName = "uid"
		}

		save := false
		for _, field := range permitted {
			if dbName == field || field == "*" {
				save = true
				break
			}
		}
		if !save {
			continue //skipping
		}

		for d := 0; d < modelVal.NumField(); d++ {
			// field := val.Field(c)
			fieldType := modelVal.Type().Field(d)
			fieldJsonName := fieldType.Tag.Get("json")
			// logging(this.Context).Debugf("SetsFromGraphQLArgs-field: %s vs %s", fieldJsonName, dbName)
			if fieldJsonName == dbName {
				logging(this.Context).Debugf("field: %s, tag: %#v = %#v", fieldType.Name, fieldType.Tag.Get("json"), field.Interface())
				foundField = true
				// modelField := modelVal.Field(d)
				if err := this.Set(fieldType.Name, field.Interface(), markChanged); err != nil {
					err2 := fmt.Errorf("Error setting %s=%+v, error: %+v", fieldType.Name, field, err)
					return err2
				}
				break
			}
		} //each model field
		if !foundField {
			logging(this.Context).Debugf("ignore missing %s", dbName)
		}
		// modelField := modelVal.FieldByName(dbName)
	} //each values field

	return nil
}
