package dgraph_test

import (
	"reflect"
	"time"

	_ "github.com/u007/gobinder"

	graphql "github.com/graph-gophers/graphql-go"
)

type TestRole struct {
	UID  string `json:"uid"`
	Name string `json:"name"`
}

func (t TestRole) TableName() string {
	return "test_roles"
}

type InputTestRole struct {
	Name string `json:"name"`

	DELETE_ bool `json:"delete_"`
}

type User struct {
	First_name string `json:"first_name"`
}

type Role struct {
	Name string `json:"name"`
}

type TestModel struct {
	UID      string    `json:"uid"`
	Name     string    `json:"name"`
	Start_at time.Time `json:"start_at"`
	Status   int       `json:"status"`

	User *User `json:"user"`

	Roles *[]Role `json:"roles"`

	TestRoles *[]TestRole `json:"test_roles"`

	SingleRole *TestRole `json:"single_role"`

	First_name         string     `json:"first_name"`
	Last_name          *string    `json:"last_name"`
	Mobile_no          *string    `json:"mobile_no"`
	Email              *string    `json:"email"`
	RegistrationCode   string     `json:"registration_code"`
	ResetExpiredAt     *time.Time `json:"reset_expired_at"`
	VerificationStatus bool       `json:"verification_status"`
	SignupType         string     `json:"signup_type"`
}

func (t TestModel) TableName() string {
	return "test_models"
}

func (a *TestSuite) TestBinderPermission() {
	tx := a.Tx

	id := "test"
	name := "aaa"
	start_at := graphql.Time{Time: time.Now()}
	status := int32(5)
	args := struct {
		ID       *string
		Name     *string
		Start_at *graphql.Time
		Status   *int32
	}{
		ID:       &id,
		Name:     &name,
		Start_at: &start_at,
		Status:   &status,
	}

	testmodel := TestModel{}
	binder := NewModelBinder(tx, a.Context, &testmodel)
	permitted := []string{"name", "start_at", "status"}

	err := binder.SetsFromGraphQLArgs(args, []string{"name"}, true)

	a.NoError(err)
	a.True(testmodel.Name == name)
	a.True(testmodel.Status == 0) //not permitted

	err = binder.SetsFromGraphQLArgs(args, permitted, true)
	a.Logger.Debugf("test model %#v", testmodel)
	a.True(testmodel.Status == 5) //not permitted
}

func (as *ModelTestSuite) TestBaseBinder() {
	tx := as.Tx
	//test Persisted and iscreate
	// var err error
	var user TestModel
	binder := model.NewModelBinder(tx, as.Context, &user)
	binder.Set("RegistrationCode", "xxx", true)
	binder.Set("SignupType", "custom", true)
	as.True(binder.Changed("RegistrationCode"))
	as.True(binder.Changed("SignupType"))
	as.False(binder.Changed("First_name"))
	signupType := binder.Changes["SignupType"] //set in init
	as.Logger.Debugf("Signup type: %#v", signupType)
	as.Equal("", signupType[0])
	as.Equal("", binder.OldValue("SignupType"))
	as.Equal("custom", signupType[1])

	var name string = "yyy"
	binder.Set("RegistrationCode", &name, true)
	as.True(user.RegistrationCode == "yyy", "should yyy")

	binder.Set("Mobile_no", nil, true)
	as.True(user.Mobile_no == nil, "Mobile no: %#v", user)
	binder.Set("Mobile_no", "12345", true)
	as.Logger.Debugf("user: %#v", user)
	as.True(*user.Mobile_no == "12345")
	binder.Set("Mobile_no", name, true)
	as.True(*user.Mobile_no == "yyy")

	binder.Set("Password", GENERIC_PASSWORD, true)

	testEmail := "test@example.com"
	as.True(!binder.Changed("Email"))
	// pass in ptr string to ptr field
	binder.Set("Email", &testEmail, true)
	as.True(binder.Changed("Email"))
	//no longer true here
	// as.True(user.Email == &testEmail, "testemail: %#v vs %#v", testEmail, *user.Email)
	as.True(*user.Email == testEmail)
	as.True(binder.Changes["Email"][0].(*string) == nil)
	as.True(*binder.Changes["Email"][1].(*string) == testEmail)

	// pass in nil to ptr field make sure changes are captured (from valid value to nil value)
	binder.Set("Email", nil, true)
	as.True(binder.Changed("Email"))
	as.True(user.Email == nil)
	as.True(*binder.Changes["Email"][0].(*string) == testEmail)
	as.True(binder.Changes["Email"][1].(*string) == nil)

	// pass in string to ptr field
	binder.Set("Email", testEmail, true)
	as.True(binder.Changed("Email"))
	as.True(user.Email != &testEmail)
	as.True(*user.Email == testEmail)
	as.True(binder.Changes["Email"][0].(*string) == nil)
	as.True(*binder.Changes["Email"][1].(*string) == testEmail)

	timeNow := time.Now()
	as.True(!binder.Changed("ResetExpiredAt"))

	// pass in ptr value to ptr field
	binder.Set("ResetExpiredAt", &timeNow, true)
	as.True(binder.Changed("ResetExpiredAt"))
	as.True(*user.ResetExpiredAt == timeNow)
	as.True(binder.Changes["ResetExpiredAt"][0].(*time.Time) == nil)
	as.True(*binder.Changes["ResetExpiredAt"][1].(*time.Time) == timeNow)

	// pass in nil to ptr field make sure changes are captured (from valid value to nil value)
	binder.Set("ResetExpiredAt", nil, true)
	as.True(binder.Changed("ResetExpiredAt"))
	as.True(user.ResetExpiredAt == nil)
	as.True(*binder.Changes["ResetExpiredAt"][0].(*time.Time) == timeNow)
	as.True(binder.Changes["ResetExpiredAt"][1].(*time.Time) == nil)

	newTime := time.Now()
	// pass in non ptr value to ptr field
	binder.Set("ResetExpiredAt", newTime, true)
	as.True(binder.Changed("ResetExpiredAt"))
	as.True(*user.ResetExpiredAt == newTime)
	as.True(binder.Changes["ResetExpiredAt"][0].(*time.Time) == nil)
	as.True(*binder.Changes["ResetExpiredAt"][1].(*time.Time) == newTime)

	_, err := SaveBinder(tx, as.Context, &binder)
	as.NoError(err)

	binder2 := model.NewModelBinder(tx, as.Context, &user2)
	as.False(binder2.Changed("SignupType"))

	var test2 TestModel
	binder = model.NewModelBinder(tx, as.Context, &test2)
	binder.SetValue("VerificationStatus", reflect.ValueOf(true), true)
	as.True(test2.VerificationStatus == true)
	binder.SetValue("VerificationStatus", reflect.ValueOf("true"), true)
	as.True(test2.VerificationStatus == true)
	binder.SetValue("VerificationStatus", reflect.ValueOf("false"), true)
	as.True(test2.VerificationStatus == false)
	binder.SetValue("VerificationStatus", reflect.ValueOf("1"), true)
	as.True(test2.VerificationStatus == true)
	binder.SetValue("VerificationStatus", reflect.ValueOf("0"), true)
	as.True(test2.VerificationStatus == false)
}
