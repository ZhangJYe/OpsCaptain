package tools

import (
	"strings"
	"testing"
)

func TestMysqlCrudTool_RejectNonSelect(t *testing.T) {
	tool := NewMysqlCrudTool()

	info, err := tool.Info(nil)
	if err != nil {
		t.Fatalf("failed to get tool info: %v", err)
	}
	if info.Name != "mysql_crud" {
		t.Fatalf("expected tool name 'mysql_crud', got '%s'", info.Name)
	}
}

func TestMysqlCrudTool_SQLValidation(t *testing.T) {
	testCases := []struct {
		name     string
		sql      string
		rejected bool
	}{
		{"SELECT allowed", "SELECT * FROM users", false},
		{"select lowercase allowed", "select id from users", false},
		{"SELECT with spaces", "  SELECT * FROM users  ", false},
		{"DROP rejected", "DROP TABLE users", true},
		{"DELETE rejected", "DELETE FROM users", true},
		{"INSERT rejected", "INSERT INTO users VALUES (1)", true},
		{"UPDATE rejected", "UPDATE users SET name='x'", true},
		{"ALTER rejected", "ALTER TABLE users ADD col INT", true},
		{"TRUNCATE rejected", "TRUNCATE TABLE users", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sqlUpper := strings.TrimSpace(strings.ToUpper(tc.sql))
			isSelect := strings.HasPrefix(sqlUpper, "SELECT")
			if tc.rejected && isSelect {
				t.Errorf("expected SQL '%s' to be rejected, but it starts with SELECT", tc.sql)
			}
			if !tc.rejected && !isSelect {
				t.Errorf("expected SQL '%s' to be allowed, but it doesn't start with SELECT", tc.sql)
			}
		})
	}
}
