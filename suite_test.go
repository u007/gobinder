package gobinder_test

import (
	"context"
	"fmt"
	"reflect"

	"testing"

	"github.com/gobuffalo/plush"
	"github.com/u007/gobinder"
	"github.com/u007/gobinder/lib"

	"github.com/stretchr/testify/suite"
)

const (
	GENERIC_PASSWORD = "abc123"
)

// Define the suite, and absorb the built-in basic suite
// functionality from testify - including assertion methods.
type TestSuite struct {
	suite.Suite
	Logger  lib.Logger
	Context *plush.Context
}

func (a TestSuite) NoError(e error) {
	if e != nil {
		panic(fmt.Sprintf("%+v", e))
	}
}

func (a TestSuite) Nil(v interface{}, msg ...interface{}) {
	if !reflect.ValueOf(v).IsValid() {
		return
	}

	if !reflect.ValueOf(v).IsNil() {
		if len(msg) > 1 {
			panic(fmt.Errorf(msg[0].(string), msg[1:]...))
		}
		if len(msg) == 1 {
			panic(msg[0].(string))
		}
		panic(fmt.Errorf("is nil"))
	}
}

func (a TestSuite) NotNil(v interface{}, msg ...interface{}) {
	if !reflect.ValueOf(v).IsValid() || reflect.ValueOf(v).IsNil() {
		if len(msg) > 1 {
			panic(fmt.Errorf(msg[0].(string), msg[1:]...))
		}
		if len(msg) == 1 {
			panic(msg[0].(string))
		}
		panic(fmt.Errorf("is nil"))
	}
}

func (a TestSuite) True(t bool, msg ...interface{}) {
	if !t {
		if len(msg) > 1 {
			panic(fmt.Errorf(msg[0].(string), msg[1:]...))
		}
		if len(msg) == 1 {
			panic(msg[0].(string))
		}
		panic(fmt.Errorf("Not true"))
	}
}

func (a TestSuite) False(t bool, msg ...interface{}) {
	if t {
		if len(msg) > 1 {
			panic(fmt.Errorf(msg[0].(string), msg[1:]...))
		}
		if len(msg) == 1 {
			panic(msg[0].(string))
		}
		panic(fmt.Errorf("Not false"))
	}
}

// Make sure that VariableThatShouldStartAtFive is set to five
// before each test
func (a *TestSuite) SetupTest() {

}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestTestSuite(t *testing.T) {
	var ctx context.Context //:= plush.NewContext()
	logger := gobinder.SetupLogging()
	ctx = context.WithValue(ctx, "log", *logger)
	testS := new(TestSuite)

	testS.Logger = *logger
	testS.Context = plush.NewContextWithContext(ctx)
	suite.Run(t, testS)
}
