package dgraph

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgo/y"
	graphql "github.com/graph-gophers/graphql-go"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var DebugSchema = true

type DGraphTxn struct {
	Tx     *dgo.Txn
	Client *dgo.Dgraph
}

func NewDGraphTxn(connection *grpc.ClientConn) *DGraphTxn {
	client := dgo.NewDgraphClient(api.NewDgraphClient(connection))
	return &DGraphTxn{Client: client, Tx: client.NewTxn()}
}

func (d *DGraphTxn) Schema(ctx context.Context, schema string) error {
	if d.Client == nil {
		return fmt.Errorf("Client not set")
	}
	op := &api.Operation{
		Schema: schema,
	}
	err := d.Client.Alter(ctx, op)
	return err
}

func (d *DGraphTxn) Discard(ctx context.Context) error {
	return d.Tx.Discard(ctx)
}

func (d *DGraphTxn) Commit(ctx context.Context) error {
	return d.Tx.Commit(ctx)
}

func (d *DGraphTxn) QueryWithVars(ctx context.Context, q string, qVars map[string]string) (*api.Response, error) {
	// logging(ctx).Debugf("vars, Q:%#v | %#v", q, qVars)
	return d.Tx.QueryWithVars(ctx, q, qVars)
}

func DeleteRelationField(tx *DGraphTxn, ctx context.Context, model interface{}, fieldName string, deleteRelation bool) error {
	val := reflect.Indirect(reflect.ValueOf(model))
	modelType := reflect.TypeOf(model)
	queryModel := reflect.New(modelType.Elem())
	// logging(ctx).Debugf("model type: %#v", modelType.Elem().Kind())
	field, ok := modelType.Elem().FieldByName(fieldName)
	if !ok {
		return fmt.Errorf("Field missing: %s", fieldName)
	}
	dbName := field.Tag.Get("json")
	if dbName == "" {
		return fmt.Errorf("Unable to get Json field %s", fieldName)
	}

	id := val.FieldByName("UID").Interface().(string)

	// logging(ctx).Debugf("new %#v", queryModel)
	if err := Q(tx).Select(QueryField{Name: "uid"}).Select(QueryField{Name: dbName}.AddField("*", nil)).
		First(ctx, queryModel.Interface()); err != nil {
		return err
	}

	childField := reflect.Indirect(queryModel.Elem().FieldByName(fieldName))
	// logging(ctx).Debugf("child found: %s: %#v | %#v", fieldName, queryModel.Elem(), childField)
	logging(ctx).Debugf("child: %s (%s) = %#v", fieldName, childField.Kind(), childField)
	//ensure is not nil
	if childField.IsValid() {
		childs := childField.Interface()
		logging(ctx).Debugf("child: %s = %#v (%s)", fieldName, childs, childField.Kind())
		if childField.Kind() == reflect.Slice {
			for i := 0; i < childField.Len(); i++ {
				row := childField.Index(i)
				logging(ctx).Debugf("Deleting multiple relation: %s %#v", dbName, row.Interface())
				if _, err := Destroy(tx, ctx, row.Addr().Interface()); err != nil {
					return err
				}
			}
		} else if childField.Kind() == reflect.Struct {
			logging(ctx).Debugf("Deleting relation: %s %#v", dbName, childField)
			if _, err := Destroy(tx, ctx, childField.Addr().Interface()); err != nil {
				return err
			}
		}
	}

	if !deleteRelation {
		return nil
	}

	if _, err := tx.MutateDeleteField(ctx, id, dbName, nil, false); err != nil {
		logging(ctx).Errorf("unable to delete field: %+v", err)
		return err
	}
	return nil
}

func (d *DGraphTxn) Save(ctx context.Context, data interface{}, commit bool) (*api.Assigned, error) {
	dataJson, err := json.Marshal(data)
	if err != nil {
		dummy := api.Assigned{}
		return &dummy, err
	}

	return d.Mutate(ctx, &api.Mutation{
		SetJson:   dataJson,
		CommitNow: commit,
	})
}

// Delete an object by id
func (d *DGraphTxn) DeleteByUID(ctx context.Context, uid string, commit bool) (*api.Assigned, error) {
	if uid == "" {
		return &api.Assigned{}, fmt.Errorf("invalid uid")
	}

	hash := map[string]string{"uid": uid}
	pb, err := json.Marshal(hash)
	if err != nil {
		return &api.Assigned{}, err
	}

	return d.Mutate(ctx, &api.Mutation{
		DeleteJson: pb,
		CommitNow:  commit,
	})
}

// https://docs.dgraph.io/mutations/#triples
func (d *DGraphTxn) MutateSet(ctx context.Context, sets []*api.NQuad, commit bool) (*api.Assigned, error) {
	return d.Mutate(ctx, &api.Mutation{
		Set:       sets,
		CommitNow: commit,
	})
}

//create relation from "id" by relation(name/predicate) to "relatedID"
func (d *DGraphTxn) Associate(ctx context.Context, id string, relation string, relatedID string, commit bool) (*api.Assigned, error) {
	val := &api.NQuad{Subject: id, Predicate: relation, ObjectId: relatedID}

	sets := []*api.NQuad{
		val,
	}

	return d.Mutate(ctx, &api.Mutation{
		Set:       sets,
		CommitNow: commit,
	})
}

// Set value for a predicate by object ID
func (d *DGraphTxn) MutateField(ctx context.Context, id string, fieldName string, value interface{}, commit bool) (*api.Assigned, error) {
	apiVal, err := parseAsApiValue(value)
	if err != nil {
		return &api.Assigned{}, err
	}

	val := &api.NQuad{Subject: id, Predicate: fieldName, ObjectValue: apiVal}

	sets := []*api.NQuad{
		val,
	}

	return d.Mutate(ctx, &api.Mutation{
		Set:       sets,
		CommitNow: commit,
	})
}

func parseAsApiValue(value interface{}) (*api.Value, error) {
	valType := reflect.TypeOf(value)
	if valType.Kind() == reflect.Slice {
		valType = valType.Elem()
		switch valType.Name() {
		case "uint8":
			return &api.Value{Val: &api.Value_BytesVal{value.([]byte)}}, nil
		}

		return &api.Value{}, fmt.Errorf("Unsupported slice %s", valType.Name())
	}
	//get value instead of ptr
	if valType.Kind() == reflect.Ptr {
		valType = valType.Elem()
		value = reflect.ValueOf(value).Elem().Interface()
	}

	switch valType.Name() {
	case "string":
		return &api.Value{Val: &api.Value_DefaultVal{value.(string)}}, nil
	case "bool":
		return &api.Value{Val: &api.Value_BoolVal{value.(bool)}}, nil
	case "int":
		return &api.Value{Val: &api.Value_IntVal{int64(value.(int))}}, nil
	case "int8":
		return &api.Value{Val: &api.Value_IntVal{int64(value.(int8))}}, nil
	case "int16":
		return &api.Value{Val: &api.Value_IntVal{int64(value.(int16))}}, nil
	case "int32":
		return &api.Value{Val: &api.Value_IntVal{int64(value.(int32))}}, nil
	case "int64":
		return &api.Value{Val: &api.Value_IntVal{value.(int64)}}, nil
	case "float32":
		return &api.Value{Val: &api.Value_DoubleVal{float64(value.(float32))}}, nil
	case "float64":
		return &api.Value{Val: &api.Value_DoubleVal{value.(float64)}}, nil
	case "Time":
		theTime := value.(time.Time)
		// return types.Convert(value, types.DateTimeID), nil
		return &api.Value{Val: &api.Value_DefaultVal{theTime.Format(time.RFC3339)}}, nil
	}
	return &api.Value{}, fmt.Errorf("Unsupported type %s", reflect.TypeOf(value).Name())
}

// ctx
//	id object id
//	predicate
//	value (nil / api.Value optional) - nil means *
//	commit bool
func (d *DGraphTxn) MutateDeleteField(ctx context.Context, id string, field string, value *api.Value, commit bool) (*api.Assigned, error) {
	val := &api.NQuad{Subject: id, Predicate: field}
	// val.ObjectValue =  &api.Value{Val: &api.Value_StrVal{StrVal: "*"}}
	if value != nil {
		val.ObjectValue = value
	} else {
		val.ObjectValue = &api.Value{Val: &api.Value_DefaultVal{"_STAR_ALL"}}
	}

	sets := []*api.NQuad{
		val,
	}

	return d.Mutate(ctx, &api.Mutation{
		Del:       sets,
		CommitNow: commit,
	})
	// d.MutateDelete(ctx, fmt.Sprintf(`"%s" "%s" * .`, id, field), commit)
}

/*
delete {
     <0xf11168064b01135b> <died> "1998" .
  }
*/
func (d *DGraphTxn) MutateDelete(ctx context.Context, raw string, commit bool) (*api.Assigned, error) {
	return d.Mutate(ctx, &api.Mutation{
		DeleteJson: []byte(raw),
		CommitNow:  commit,
	})
}

func (d *DGraphTxn) Mutate(ctx context.Context, mu *api.Mutation) (*api.Assigned, error) {
	assigned, err := d.Tx.Mutate(ctx, mu)
	return assigned, err
}

func (d *DGraphTxn) Transact(ctx context.Context) error {
	trial, maxTrial := 1, 3
	for {
		err := d.Tx.Commit(ctx)
		if err == y.ErrAborted {
			// Retry or handle error
			trial++
			if trial > maxTrial {
				return err
			}
		} else {
			return err
		}
	}
	return nil
}

func UpdateSchema(tx *DGraphTxn, ctx context.Context, model interface{}, commit bool) error {
	var tableName string
	if table, ok := model.(TableNameAble); !ok {
		return fmt.Errorf("Not a table")
	} else {
		tableName = table.TableName()
	}

	// logging(ctx).Debugf("kind %+v", reflect.ValueOf(model).Kind())
	var modelType reflect.Type
	if reflect.ValueOf(model).Kind() == reflect.Ptr {
		modelType = reflect.TypeOf(model).Elem()

	} else {
		modelType = reflect.TypeOf(model)
	}
	record := reflect.New(modelType).Elem()

	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		fieldName := field.Name
		fieldType := field.Type
		if fieldName == "UID" {
			continue //skip
		}

		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}

		if fieldType.Kind() == reflect.Slice {
			if DebugSchema {
				logging(ctx).Debugf("Skipping slice %s", fieldName)
			}
			continue // skip
		}

		if fieldType.Kind() == reflect.Struct && fieldType.String() != "time.Time" {
			if DebugSchema {
				logging(ctx).Debugf("Skipping struct %s", fieldName)
			}
			continue //skip
		}

		dbFieldName := field.Tag.Get("json")
		if dbFieldName == "" {
			return fmt.Errorf("Missing json tag for field %s of %s", fieldName, tableName)
		}

		// logging(ctx).Debugf("field: %s - %s", fieldName, dbFieldName)
		for {

			slice := reflect.MakeSlice(reflect.SliceOf(modelType), 0, 0)
			rowsAddr := reflect.New(slice.Type())
			rowsAddr.Elem().Set(slice)
			// fmt.Printf("point: %#v\n", rowsAddr.Interface())
			// rows := rowsAddr.Elem()
			// rowsAddr.Set(slice)
			rows := rowsAddr.Elem()
			// rows.Set(reflect.Append(rows, reflect.New(modelType).Elem()))
			// logging(ctx).Debugf("rows %#v", rowsAddr.Interface())

			query := Q(tx).Where(fmt.Sprintf("NOT has(%s)", dbFieldName), map[string]QVar{}).Paginate(100, 1).
				Select(QueryField{Name: "uid"})
			if err := query.All(ctx, rowsAddr.Interface()); err != nil {

				return fmt.Errorf("Unable to get %s of collection %s, err: %+v", fieldName, tableName, err)
			} else {
				if rows.Len() == 0 {
					break //no more record
				}

				// logging(ctx).Debugf("Found (%d) without %s.%s", rows.Len(), tableName, fieldName)
				fieldValue := record.FieldByName(fieldName)
				for r := 0; r < rows.Len(); r++ {
					id := rows.Index(r).FieldByName("UID").Interface().(string)

					if fieldValue.Kind() == reflect.Ptr {
						fieldValue = fieldValue.Elem()
					}
					// logging(ctx).Debugf("field type: %v, kind: %v", fieldValue.Type().Name(), fieldValue.Type().Kind())
					// logging(ctx).Infof("Updating %s id: %s, field: %s=%+v", tableName, id, dbFieldName, fieldValue.Interface())
					if _, err := tx.MutateField(ctx, id, dbFieldName, fieldValue.Interface(), commit); err != nil {
						return fmt.Errorf("Unable to update %s id: %s, field: %s=%+v, err: %+v", tableName, id, fieldName, fieldValue.Interface(), err)
					}
				} //each rows

			} //found records without this column
		} //while has records without this column

	} //end each field

	return nil
}

func EncodeCursor(i *string) graphql.ID {
	id := "cursor"
	if i != nil {
		id += *i
	}
	return graphql.ID(base64.StdEncoding.EncodeToString([]byte(id)))
}

func DecodeCursor(after *string) (*string, error) {
	var decodedValue string
	if after != nil {
		b, err := base64.StdEncoding.DecodeString(*after)
		if err != nil {
			return nil, err
		}
		i := strings.TrimPrefix(string(b), "cursor")
		decodedValue = i
	}
	return &decodedValue, nil
}
