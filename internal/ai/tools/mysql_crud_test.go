package tools

import "testing"

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

func TestValidateMysqlQuery(t *testing.T) {
	policy := mysqlQueryPolicy{maxRows: 100}
	testCases := []struct {
		name    string
		sql     string
		wantErr bool
	}{
		{name: "SELECT allowed", sql: "SELECT * FROM users"},
		{name: "select lowercase allowed", sql: "select id from users"},
		{name: "SELECT with spaces", sql: "  SELECT * FROM users  "},
		{name: "DROP rejected", sql: "DROP TABLE users", wantErr: true},
		{name: "DELETE rejected", sql: "DELETE FROM users", wantErr: true},
		{name: "INSERT rejected", sql: "INSERT INTO users VALUES (1)", wantErr: true},
		{name: "UPDATE rejected", sql: "UPDATE users SET name='x'", wantErr: true},
		{name: "ALTER rejected", sql: "ALTER TABLE users ADD col INT", wantErr: true},
		{name: "TRUNCATE rejected", sql: "TRUNCATE TABLE users", wantErr: true},
		{name: "multi statements rejected", sql: "SELECT * FROM users; DROP TABLE users", wantErr: true},
		{name: "sleep rejected", sql: "SELECT SLEEP(10)", wantErr: true},
		{name: "comment bypass rejected", sql: "SELECT * FROM users dr/**/op table test", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			query, err := validateMysqlQuery(tc.sql, policy)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q, got query %q", tc.sql, query)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("did not expect error for %q: %v", tc.sql, err)
			}
			if !tc.wantErr && query == "" {
				t.Fatalf("expected wrapped query for %q", tc.sql)
			}
		})
	}
}

func TestValidateMysqlQuery_Allowlist(t *testing.T) {
	policy := mysqlQueryPolicy{
		maxRows: 100,
		allowedTables: map[string]struct{}{
			"orders": {},
		},
	}

	if _, err := validateMysqlQuery("SELECT * FROM orders", policy); err != nil {
		t.Fatalf("expected orders table to be allowed: %v", err)
	}
	if _, err := validateMysqlQuery("SELECT * FROM users", policy); err == nil {
		t.Fatal("expected users table to be rejected by allowlist")
	}
}

func TestValidateMysqlQuery_SubqueryBypass(t *testing.T) {
	policy := mysqlQueryPolicy{
		maxRows: 100,
		allowedTables: map[string]struct{}{
			"orders": {},
		},
	}

	bypassAttempts := []struct {
		name string
		sql  string
	}{
		{"SELECT subquery", "SELECT (SELECT password FROM users LIMIT 1) FROM orders"},
		{"WHERE IN subquery", "SELECT * FROM orders WHERE user_id IN (SELECT id FROM users)"},
		{"WHERE EXISTS subquery", "SELECT * FROM orders WHERE EXISTS (SELECT 1 FROM users WHERE users.id = orders.user_id)"},
		{"FROM subquery", "SELECT * FROM (SELECT * FROM users) AS u"},
		{"JOIN subquery", "SELECT * FROM orders JOIN (SELECT id FROM users) AS u ON orders.user_id = u.id"},
	}

	for _, tc := range bypassAttempts {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateMysqlQuery(tc.sql, policy)
			if err == nil {
				t.Fatalf("expected subquery to be rejected: %q", tc.sql)
			}
		})
	}

	// Subqueries should be allowed when no allowlist is configured
	openPolicy := mysqlQueryPolicy{maxRows: 100}
	if _, err := validateMysqlQuery("SELECT * FROM (SELECT * FROM users) AS u", openPolicy); err != nil {
		t.Fatalf("subquery should be allowed without allowlist: %v", err)
	}
}
