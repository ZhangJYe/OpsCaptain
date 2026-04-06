package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/gogf/gf/v2/frame/g"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type MysqlCrudInput struct {
	SQL         string `json:"sql" jsonschema:"description=The SQL SELECT query to execute against the MySQL database. Only SELECT statements are allowed for safety."`
	OperateType string `json:"operate_type" jsonschema:"description=The type of SQL operation to perform. Currently only 'query' is supported."`
}

var (
	mysqlDB   *gorm.DB
	mysqlOnce sync.Once
	mysqlErr  error
)

func initMysqlDB(ctx context.Context) (*gorm.DB, error) {
	mysqlOnce.Do(func() {
		dsn, err := g.Cfg().Get(ctx, "mysql.dsn")
		if err != nil {
			mysqlErr = fmt.Errorf("failed to read mysql.dsn from config: %w", err)
			return
		}
		if dsn.String() == "" {
			mysqlErr = fmt.Errorf("mysql.dsn is not configured")
			return
		}
		db, err := gorm.Open(mysql.Open(dsn.String()), &gorm.Config{})
		if err != nil {
			mysqlErr = fmt.Errorf("failed to connect to mysql: %w", err)
			return
		}
		mysqlDB = db
	})
	return mysqlDB, mysqlErr
}

func NewMysqlCrudTool() tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"mysql_crud",
		"Execute read-only SQL SELECT queries against the MySQL database and return results in JSON format. Only single SELECT statements are allowed. Use this tool when you need to query data from the database.",
		func(ctx context.Context, input *MysqlCrudInput, opts ...tool.Option) (output string, err error) {
			sqlUpper := strings.TrimSpace(strings.ToUpper(input.SQL))
			if !strings.HasPrefix(sqlUpper, "SELECT") {
				return `{"success":false,"error":"only SELECT queries are allowed"}`, nil
			}
			if strings.Contains(input.SQL, ";") {
				return `{"success":false,"error":"multiple statements are not allowed"}`, nil
			}
			for _, kw := range []string{"INTO OUTFILE", "INTO DUMPFILE", "SLEEP(", "BENCHMARK(", "LOAD_FILE("} {
				if strings.Contains(sqlUpper, kw) {
					return `{"success":false,"error":"forbidden SQL keyword detected"}`, nil
				}
			}

			db, err := initMysqlDB(ctx)
			if err != nil {
				return fmt.Sprintf(`{"success":false,"error":"MySQL is not available: %s"}`, err.Error()), nil
			}

			var results []map[string]interface{}
			err = db.Raw(input.SQL).Scan(&results).Error
			if err != nil {
				return fmt.Sprintf(`{"success":false,"error":"query failed: %s"}`, err.Error()), nil
			}

			resBytes, err := json.Marshal(results)
			if err != nil {
				return "", fmt.Errorf("failed to marshal results: %w", err)
			}
			return string(resBytes), nil
		})
	if err != nil {
		panic(fmt.Sprintf("failed to create mysql_crud tool: %v", err))
	}
	return t
}
