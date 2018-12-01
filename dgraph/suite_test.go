package dgraph_test

import (
	"context"
	"fmt"
	"net/http"

	"reflect"
	"strings"

	_ "github.com/u007/gobinder/dgraph"
	"github.com/u007/gobinder/lib"

	"testing"

	// "fmt"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/gobuffalo/plush"
	"google.golang.org/grpc"

	"github.com/stretchr/testify/suite"
)

const (
	GENERIC_PASSWORD = "aaaaaa"
)

// Define the suite, and absorb the built-in basic suite
// functionality from testify - including assertion methods.
type TestSuite struct {
	suite.Suite
	Logger   lib.Logger
	Context  *plush.Context
	Tx       *DGraphTxn
	DBCon    *grpc.ClientConn
	DBClient *dgo.Dgraph
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
	if a.DBCon == nil {
		panic("db nil")
	}

	//commit last transaction so we can look at data
	if a.Tx != nil {
		if err := a.Tx.Commit(a.Context); err != nil {
			panic(err)
		}
	}

	a.Tx = NewDGraphTxn(a.DBCon)

	body := strings.NewReader(`{"drop_all": true}`)
	req, err := http.NewRequest("POST", "http://"+g.IniGet("db.admin.host").(string)+":"+g.IniGet("db.admin.port").(string)+"/alter", body)
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// handle err
		panic(err)
	}
	defer resp.Body.Close()

	if err := dgraph.DBSchemaLoad(a.DBClient, a.Tx, a.Context); err != nil {
		panic(err)
	}

	if err := a.Tx.Commit(a.Context); err != nil {
		panic(err)
	}

	a.Tx = NewDGraphTxn(a.DBCon)
}

func (a *TestSuite) MustCommitTx() *DGraphTxn {
	err := a.CommitTx()
	a.NoError(err)
	return a.Tx
}

//commit the transaction for data checking
// ensure to use latest a.Tx after this
func (a *TestSuite) CommitTx() error {
	if err := a.Tx.Commit(a.Context); err != nil {
		return err
	}

	a.Tx = NewDGraphTxn(a.DBCon)
	return nil
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestTestSuite(t *testing.T) {
	// model.DebugQuery = false
	// model.DebugSchema = false
	var ctx context.Context //:= plush.NewContext()
	testS := new(TestSuite)

	rootPath := ".."
	config := g.LoadConfig(rootPath + "/Config.toml")

	var err error
	if ctx, err = g.SetupContext(config, rootPath); err != nil {
		panic(err)
	}

	testS.Logger = *ctx.Value("log").(*lib.Logger)
	dbConn := ctx.Value("db").(*grpc.ClientConn)
	testS.DBCon = dbConn
	testS.DBClient = dgo.NewDgraphClient(api.NewDgraphClient(dbConn))

	testS.Context = plush.NewContextWithContext(ctx)
	// fmt.Printf("testSuite %#v\n", testS)
	suite.Run(t, testS)
}
