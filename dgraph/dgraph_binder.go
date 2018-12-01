package dgraph

import (
	"context"
	"fmt"
	"reflect"
	"time"

	_ "github.com/u007/gobinder"

	"github.com/fatih/structs"
	"github.com/gobuffalo/validate"
)

type BinderBaseModel interface {
	BinderInit(*DGraphTxn, context.Context, *ModelBinder) error
}

type BasePreValidatable interface {
	PreValidate(*DGraphTxn, context.Context, string, *ModelBinder) error
}

type BaseBinderValidatable interface {
	BinderValidate(*DGraphTxn, context.Context, string, *ModelBinder) (*validate.Errors, error)
}

type BaseAfterSavable interface {
	AfterSave(*DGraphTxn, context.Context, string, *ModelBinder) error
}
type BaseAfterSaveableRelation interface {
	AfterSaveRelation(*DGraphTxn, context.Context, *ModelBinder, ...string) error
}

type BaseBeforeDestroy interface {
	BeforeDestroy(*DGraphTxn, context.Context) error
}

type BaseAfterDestroy interface {
	AfterDestroy(*DGraphTxn, context.Context) error
}

// @param model -> class only, exclude value
// @param map[string]interface for values
//only accept 1 values, as optional
func NewModelBinder(tx *DGraphTxn, ctx context.Context, model interface{}, values ...map[string]interface{}) ModelBinder {
	binder := ModelBinder{model: model, Context: ctx}
	binder.Changes = map[string][]interface{}{}

	valueObj := reflect.ValueOf(model).Elem()
	if valueObj.FieldByName("UID").Interface().(string) == "" {
		//is new
		yesNew := true
		binder.ModelNew = &yesNew

		if binderModel, ok := model.(BinderBaseModel); ok {
			binderModel.BinderInit(tx, ctx, &binder)
		}
	}

	if len(values) > 0 {
		binder.Sets(values[0], true)
	}
	return binder
}

func NewModelBinderAddr(tx *DGraphTxn, ctx context.Context, model interface{}, values ...map[string]interface{}) *ModelBinder {
	binder := NewModelBinder(tx, ctx, model, values...)
	return &binder
}

type SaveCallBack func(*ModelBinder) error

type SaveHelper struct {
	tx          *DGraphTxn
	ctx         context.Context
	dataModel   interface{}
	values      map[string]interface{}
	where       string
	whereValues map[string]QVar
	queryFields []QueryField

	onCreate  *SaveCallBack
	onCreated *SaveCallBack
	onLoad    *SaveCallBack
}

func (s *SaveHelper) On(event string, callback SaveCallBack) *SaveHelper {
	switch event {
	case "create":
		s.onCreate = &callback
	case "created":
		s.onCreated = &callback
	case "load":
		s.onLoad = &callback
	default:
		panic(fmt.Errorf("unknown event: %s", event))
	}
	return s
}

func (s *SaveHelper) Exec() (bool, *validate.Errors, error) {
	tx := s.tx
	ctx := s.ctx
	dataModel := s.dataModel
	values := s.values
	where := s.where
	whereValues := s.whereValues
	queryFields := s.queryFields

	newValue := reflect.New(reflect.ValueOf(dataModel).Elem().Type())
	newModel := newValue.Interface()

	query := Q(tx)
	if len(queryFields) == 0 {
		query.Select(QueryField{Name: "**"})
	} else {
		for i := range queryFields {
			query.Select(queryFields[i])
		}
	}

	if err := query.Clone().Where(where, whereValues).First(ctx, newModel); err == nil {
		// val := reflect.ValueOf(dataModel)
		// logging(ctx).Debugf("found existing: %#v", val)
		binder := NewModelBinder(tx, ctx, newModel, values)
		reflect.ValueOf(dataModel).Elem().Set(reflect.ValueOf(newModel).Elem())
		if s.onLoad != nil {
			if err := (*s.onLoad)(&binder); err != nil {
				return false, nil, err
			}
		}
		// logging(ctx).Debugf("found existing: %#v", dataModel)
		return false, nil, nil
	} else if err != ErrNotFound {
		logging(ctx).Errorf("error: %+v\n", err)
		return false, nil, err
	}

	binder := NewModelBinder(tx, ctx, newModel, values)
	if s.onCreate != nil {
		if err := (*s.onCreate)(&binder); err != nil {
			return false, nil, err
		}
	}

	verrs, err := SaveBinder(tx, ctx, &binder)
	if err != nil {
		return false, verrs, err
	}

	id := reflect.ValueOf(newModel).Elem().FieldByName("UID").Interface().(string)
	logging(ctx).Debugf("looking up id %s for model %#v", id, newModel)
	if err := query.Find(ctx, newModel, id); err != nil {
		return false, verrs, err
	}

	reflect.ValueOf(dataModel).Elem().Set(reflect.ValueOf(binder.Model()).Elem())

	if s.onCreated != nil {
		if err := (*s.onCreated)(&binder); err != nil {
			return false, nil, err
		}
	}

	return true, verrs, err
}

/*
	@param tx
	@param ctx
	@param modelbinder
	@param string the where condition
	@param variables for the where condition
	@param queryFields - to obtain if exists
	@return model, created t/f, validation errors, error
*/
func SaveIfNotExist(tx *DGraphTxn, ctx context.Context, dataModel interface{}, values map[string]interface{},
	where string, whereValues map[string]QVar, queryFields ...QueryField) *SaveHelper {

	sHelper := SaveHelper{tx: tx, ctx: ctx, dataModel: dataModel, values: values,
		where: where, whereValues: whereValues, queryFields: queryFields}
	return &sHelper
}

func (s *SaveHelper) Create() (*validate.Errors, error) {
	tx := s.tx
	ctx := s.ctx
	dataModel := s.dataModel
	values := s.values
	queryFields := s.queryFields

	newValue := reflect.New(reflect.ValueOf(dataModel).Elem().Type())
	newModel := newValue.Interface()

	query := Q(tx)
	if len(queryFields) == 0 {
		query.Select(QueryField{Name: "**"})
	} else {
		for i := range queryFields {
			query.Select(queryFields[i])
		}
	}

	binder := NewModelBinder(tx, ctx, newModel, values)
	if s.onCreate != nil {
		if err := (*s.onCreate)(&binder); err != nil {
			return nil, err
		}
	}

	verrs, err := SaveBinder(tx, ctx, &binder)
	if err != nil {
		return verrs, err
	}

	id := reflect.ValueOf(newModel).Elem().FieldByName("UID").Interface().(string)
	if err := query.Find(ctx, newModel, id); err != nil {
		return nil, err
	}

	reflect.ValueOf(dataModel).Elem().Set(reflect.ValueOf(binder.Model()).Elem())
	if s.onCreated != nil {
		if err := (*s.onCreated)(&binder); err != nil {
			return nil, err
		}
	}

	return verrs, err
}

func Save(tx *DGraphTxn, ctx context.Context, dataModel interface{}, values map[string]interface{}, queryFields ...QueryField) *SaveHelper {
	sHelper := SaveHelper{tx: tx, ctx: ctx, dataModel: dataModel, values: values,
		queryFields: queryFields}
	return &sHelper
}

func SaveBinder(tx *DGraphTxn, ctx context.Context, binder *ModelBinder) (*validate.Errors, error) {
	vEmptyErrors := validate.NewErrors()
	var action string
	model := binder.Model()
	value := reflect.ValueOf(model).Elem()
	// field := value.FieldByName("ID")
	id := value.FieldByName("UID").Interface().(string)
	if id == "" {
		action = "create"
		if SetCreatedUpdatedTimeOnSave {
			thetime := time.Now().UTC()
			binder.Set("UpdatedAt", thetime, true)
			binder.Set("CreatedAt", thetime, true)
		}

	} else {
		action = "update"
		if SetCreatedUpdatedTimeOnSave {
			binder.Set("UpdatedAt", time.Now().UTC(), true)
		}
	}

	hook1, ok1 := model.(BasePreValidatable)
	if ok1 {
		if err := hook1.PreValidate(tx, ctx, action, binder); err != nil {
			return vEmptyErrors, err //return original error
		}
	}

	hookBinder, ok1 := model.(BaseBinderValidatable)
	if ok1 {
		verrs, err := hookBinder.BinderValidate(tx, ctx, action, binder)

		if err != nil {
			// gcontext.Logger.Errorf(fmt.Sprintf("BinderValidate error: %+v", err))
			return verrs, err //return original error
		}
		if verrs.HasAny() {
			return verrs, fmt.Errorf("validation error: %#v", verrs.Errors) //return new error
		}
	}

	structT := structs.New(model)
	structT.TagName = "json"
	hash := map[string]interface{}{}
	if err := ForEachField(model, func(i int, field *reflect.Value, stField reflect.StructField) error {
		// logging(ctx).Debugf("field: %#v", stField)
		if IsStructOrIsSlicesOfStruct(field.Interface()) {
			return nil
		}
		jsonName := stField.Tag.Get("json")
		hash[jsonName] = field.Interface()

		return nil
	}); err != nil {
		return vEmptyErrors, err
	}
	if len(hash) == 0 {
		return vEmptyErrors, fmt.Errorf("Nothing to save")
	}
	// hash := structT.Map()
	hash["_type"] = model.(TableNameAble).TableName()
	if DEBUG_MUTATION {
		logging(ctx).Debugf("Saving-hash: %#v", hash)
	}
	// dataJson, err := json.Marshal(hash)
	// if err != nil {
	// 	return vEmptyErrors, err
	// }
	assigned, err := tx.Save(ctx, hash, false)
	if err != nil {
		return vEmptyErrors, err
	}

	if binder.Get("UID") == "" {
		refValue := reflect.Indirect(reflect.ValueOf(model))
		refValue.FieldByName("UID").SetString(assigned.Uids["blank-0"])
		// gcontext.Logger.Debugf("new uid: %+v", model)
	}

	hook2, ok2 := model.(BaseAfterSavable)
	if ok2 {
		if err := hook2.AfterSave(tx, ctx, action, binder); err != nil {
			// gcontext.Logger.Errorf(fmt.Sprintf("postsave error: %+v", err))
			return vEmptyErrors, err //return original error
		}
	}

	return vEmptyErrors, nil
}
