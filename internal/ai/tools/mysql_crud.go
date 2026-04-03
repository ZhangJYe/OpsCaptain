package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

var mysqlDB *gorm.DB

func initMysqlDB(ctx context.Context) (*gorm.DB, error) {
	if mysqlDB != nil {
		return mysqlDB, nil
	}
	dsn, err := g.Cfg().Get(ctx, "mysql.dsn")
	if err != nil {
		return nil, fmt.Errorf("failed to read mysql.dsn from config: %w", err)
	}
	if dsn.String() == "" {
		return nil, fmt.Errorf("mysql.dsn is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn.String()), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to mysql: %w", err)
	}
	mysqlDB = db
	return mysqlDB, nil
}

func NewMysqlCrudTool() tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"mysql_crud",
		"Execute read-only SQL SELECT queries against the MySQL database and return results in JSON format. Only SELECT statements are allowed. Use this tool when you need to query data from the database.",
		func(ctx context.Context, input *MysqlCrudInput, opts ...tool.Option) (output string, err error) {
			sqlUpper := strings.TrimSpace(strings.ToUpper(input.SQL))
			if !strings.HasPrefix(sqlUpper, "SELECT") {
				return "", fmt.Errorf("only SELECT queries are allowed, got: %s", strings.Split(sqlUpper, " ")[0])
			}

			db, err := initMysqlDB(ctx)
			if err != nil {
				return "", err
			}

			var results []map[string]interface{}
			err = db.Raw(input.SQL).Scan(&results).Error
			if err != nil {
				return "", fmt.Errorf("failed to execute query: %w", err)
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
