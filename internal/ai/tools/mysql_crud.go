package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

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

	blockCommentPattern = regexp.MustCompile(`(?s)/\*.*?\*/`)
	lineCommentPattern  = regexp.MustCompile(`(?m)(--|#)[^\n]*$`)
	singleQuotePattern  = regexp.MustCompile(`'([^'\\]|\\.|'')*'`)
	doubleQuotePattern  = regexp.MustCompile(`"([^"\\]|\\.|"")*"`)
	tablePattern        = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+([a-zA-Z0-9_` + "`" + `.]+)`)
	forbiddenPatterns   = []*regexp.Regexp{
		regexp.MustCompile(`\bDROP\b`),
		regexp.MustCompile(`\bDELETE\b`),
		regexp.MustCompile(`\bUPDATE\b`),
		regexp.MustCompile(`\bINSERT\b`),
		regexp.MustCompile(`\bALTER\b`),
		regexp.MustCompile(`\bTRUNCATE\b`),
		regexp.MustCompile(`\bREPLACE\b`),
		regexp.MustCompile(`\bCREATE\b`),
		regexp.MustCompile(`\bGRANT\b`),
		regexp.MustCompile(`\bREVOKE\b`),
		regexp.MustCompile(`\bCALL\b`),
		regexp.MustCompile(`\bINTO\s+OUTFILE\b`),
		regexp.MustCompile(`\bINTO\s+DUMPFILE\b`),
		regexp.MustCompile(`\bLOAD_FILE\s*\(`),
		regexp.MustCompile(`\bSLEEP\s*\(`),
		regexp.MustCompile(`\bBENCHMARK\s*\(`),
		regexp.MustCompile(`\bFOR\s+UPDATE\b`),
		regexp.MustCompile(`\bLOCK\s+IN\s+SHARE\s+MODE\b`),
	}
)

const (
	defaultMySQLMaxRows      = 100
	defaultMySQLQueryTimeout = 10 * time.Second
)

type mysqlQueryPolicy struct {
	allowedTables map[string]struct{}
	maxRows       int
	timeout       time.Duration
}

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
			policy := loadMySQLQueryPolicy(ctx)
			query, err := validateMysqlQuery(input.SQL, policy)
			if err != nil {
				return fmt.Sprintf(`{"success":false,"error":%q}`, err.Error()), nil
			}

			db, err := initMysqlDB(ctx)
			if err != nil {
				return fmt.Sprintf(`{"success":false,"error":"MySQL is not available: %s"}`, err.Error()), nil
			}

			queryCtx, cancel := context.WithTimeout(ctxOrBackground(ctx), policy.timeout)
			defer cancel()

			var results []map[string]interface{}
			err = db.WithContext(queryCtx).Raw(query).Scan(&results).Error
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

func CloseMySQL() error {
	if mysqlDB == nil {
		return nil
	}
	sqlDB, err := mysqlDB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func loadMySQLQueryPolicy(ctx context.Context) mysqlQueryPolicy {
	policy := mysqlQueryPolicy{
		allowedTables: loadAllowedTables(ctx),
		maxRows:       defaultMySQLMaxRows,
		timeout:       defaultMySQLQueryTimeout,
	}

	if v, err := g.Cfg().Get(ctxOrBackground(ctx), "mysql.max_rows"); err == nil && v.Int() > 0 {
		policy.maxRows = v.Int()
	}
	if v, err := g.Cfg().Get(ctxOrBackground(ctx), "mysql.query_timeout_ms"); err == nil && v.Int64() > 0 {
		policy.timeout = time.Duration(v.Int64()) * time.Millisecond
	}
	return policy
}

func loadAllowedTables(ctx context.Context) map[string]struct{} {
	v, err := g.Cfg().Get(ctxOrBackground(ctx), "mysql.allowed_tables")
	if err != nil || v.IsNil() {
		return nil
	}
	items := v.Strings()
	if len(items) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(items))
	for _, item := range items {
		name := normalizeTableName(item)
		if name == "" {
			continue
		}
		allowed[name] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil
	}
	return allowed
}

func validateMysqlQuery(sql string, policy mysqlQueryPolicy) (string, error) {
	trimmed := strings.TrimSpace(sql)
	if trimmed == "" {
		return "", fmt.Errorf("sql is required")
	}
	trimmed = strings.TrimSuffix(trimmed, ";")
	if strings.Contains(trimmed, ";") {
		return "", fmt.Errorf("multiple statements are not allowed")
	}

	inspected := sanitizeSQLForInspection(trimmed)
	upper := strings.ToUpper(strings.TrimSpace(inspected))
	if !strings.HasPrefix(upper, "SELECT") {
		return "", fmt.Errorf("only SELECT queries are allowed")
	}

	for _, forbidden := range forbiddenPatterns {
		if forbidden.MatchString(upper) {
			return "", fmt.Errorf("forbidden SQL keyword detected")
		}
	}

	if len(policy.allowedTables) > 0 {
		for _, tableName := range referencedTableNames(inspected) {
			if _, ok := policy.allowedTables[tableName]; !ok {
				return "", fmt.Errorf("table %q is not in the allowlist", tableName)
			}
		}
	}

	return fmt.Sprintf("SELECT * FROM (%s) AS oncallai_safe_query LIMIT %d", trimmed, policy.maxRows), nil
}

func sanitizeSQLForInspection(sql string) string {
	out := blockCommentPattern.ReplaceAllString(sql, "")
	out = lineCommentPattern.ReplaceAllString(out, "")
	out = singleQuotePattern.ReplaceAllString(out, "''")
	out = doubleQuotePattern.ReplaceAllString(out, `""`)
	return out
}

func referencedTableNames(sql string) []string {
	matches := tablePattern.FindAllStringSubmatch(sql, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(matches))
	tables := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := normalizeTableName(match[1])
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		tables = append(tables, name)
	}
	return tables
}

func normalizeTableName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Trim(name, "`")
	if name == "" {
		return ""
	}
	if parts := strings.Split(name, "."); len(parts) > 0 {
		name = parts[len(parts)-1]
	}
	return strings.ToLower(strings.Trim(name, "`"))
}

func ctxOrBackground(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}
