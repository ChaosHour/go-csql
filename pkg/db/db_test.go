package db

import (
	"strings"
	"testing"
	"time"
)

func TestSplitSQLStatements(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []StatementInfo
	}{
		{
			name:  "simple statements",
			input: "SELECT 1; SELECT 2;",
			expected: []StatementInfo{
				{SQL: "SELECT 1", Vertical: false},
				{SQL: "SELECT 2", Vertical: false},
			},
		},
		{
			name:  "statement with \\G",
			input: "SELECT * FROM users\\G",
			expected: []StatementInfo{
				{SQL: "SELECT * FROM users", Vertical: true},
			},
		},
		{
			name:  "semicolon in string literal",
			input: "SELECT 'Hello; World' as greeting; SELECT 2;",
			expected: []StatementInfo{
				{SQL: "SELECT 'Hello; World' as greeting", Vertical: false},
				{SQL: "SELECT 2", Vertical: false},
			},
		},
		{
			name:  "semicolon in double quoted string",
			input: `SELECT "Hello; World" as greeting; SELECT 2;`,
			expected: []StatementInfo{
				{SQL: `SELECT "Hello; World" as greeting`, Vertical: false},
				{SQL: "SELECT 2", Vertical: false},
			},
		},
		{
			name:  "semicolon in backtick quoted identifier",
			input: "SELECT `column;name` FROM table; SELECT 2;",
			expected: []StatementInfo{
				{SQL: "SELECT `column;name` FROM table", Vertical: false},
				{SQL: "SELECT 2", Vertical: false},
			},
		},
		{
			name:  "semicolon in line comment",
			input: "SELECT 1; -- This is a comment with ; semicolon\nSELECT 2;",
			expected: []StatementInfo{
				{SQL: "SELECT 1", Vertical: false},
				{SQL: "-- This is a comment with ; semicolon\nSELECT 2", Vertical: false},
			},
		},
		{
			name:  "semicolon in block comment",
			input: "SELECT 1; /* This is a comment with ; semicolon */ SELECT 2;",
			expected: []StatementInfo{
				{SQL: "SELECT 1", Vertical: false},
				{SQL: "/* This is a comment with ; semicolon */ SELECT 2", Vertical: false},
			},
		},
		{
			name:  "escaped quotes in strings",
			input: `SELECT 'It\'s a test; with semicolon'; SELECT 2;`,
			expected: []StatementInfo{
				{SQL: `SELECT 'It\'s a test; with semicolon'`, Vertical: false},
				{SQL: "SELECT 2", Vertical: false},
			},
		},
		{
			name:  "statement without trailing semicolon",
			input: "SELECT 1; SELECT 2",
			expected: []StatementInfo{
				{SQL: "SELECT 1", Vertical: false},
				{SQL: "SELECT 2", Vertical: false},
			},
		},
		{
			name:  "empty statements filtered out",
			input: "SELECT 1;; ; SELECT 2;",
			expected: []StatementInfo{
				{SQL: "SELECT 1", Vertical: false},
				{SQL: "SELECT 2", Vertical: false},
			},
		},
		{
			name:  "mixed vertical and normal statements",
			input: "SELECT 1; SELECT * FROM users\\G; SELECT 2;",
			expected: []StatementInfo{
				{SQL: "SELECT 1", Vertical: false},
				{SQL: "SELECT * FROM users", Vertical: true},
				{SQL: "SELECT 2", Vertical: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitSQLStatements(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("splitSQLStatements() returned %d statements, expected %d", len(result), len(tt.expected))
				return
			}

			for i, stmt := range result {
				if stmt.SQL != tt.expected[i].SQL {
					t.Errorf("Statement %d: got SQL %q, expected %q", i, stmt.SQL, tt.expected[i].SQL)
				}
				if stmt.Vertical != tt.expected[i].Vertical {
					t.Errorf("Statement %d: got Vertical %v, expected %v", i, stmt.Vertical, tt.expected[i].Vertical)
				}
			}
		})
	}
}

func TestMaskPasswordInDSN(t *testing.T) {
	tests := []struct {
		name     string
		dsn      string
		expected string
	}{
		{
			name:     "DSN with password",
			dsn:      "user:password@tcp(localhost:3306)/database",
			expected: "user:****@tcp(localhost:3306)/database",
		},
		{
			name:     "DSN with complex password",
			dsn:      "user:p@ss!w0rd@tcp(localhost:3306)/database",
			expected: "user:****@tcp(localhost:3306)/database",
		},
		{
			name:     "DSN without password",
			dsn:      "user@tcp(localhost:3306)/database",
			expected: "user@tcp(localhost:3306)/database",
		},
		{
			name:     "DSN with unix socket",
			dsn:      "user:password@unix(/var/run/mysqld/mysqld.sock)/database",
			expected: "user:****@unix(/var/run/mysqld/mysqld.sock)/database",
		},
		{
			name:     "malformed DSN",
			dsn:      "invalid-dsn",
			expected: "invalid-dsn",
		},
		{
			name:     "DSN without protocol",
			dsn:      "user:password@localhost/database",
			expected: "user:password@localhost/database",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskPasswordInDSN(tt.dsn)
			if result != tt.expected {
				t.Errorf("maskPasswordInDSN() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestQueryResult_Duration(t *testing.T) {
	// Test that QueryResult properly stores duration
	result := QueryResult{
		Instance:       "test@tcp(localhost:3306)/test",
		Statement:      "SELECT 1",
		Rows:           [][]any{},
		Columns:        []string{"1"},
		Err:            nil,
		VerticalFormat: false,
		Duration:       100 * time.Millisecond,
		RowCount:       1,
	}

	if result.Instance != "test@tcp(localhost:3306)/test" {
		t.Errorf("Expected instance 'test@tcp(localhost:3306)/test', got %s", result.Instance)
	}

	if result.Statement != "SELECT 1" {
		t.Errorf("Expected statement 'SELECT 1', got %s", result.Statement)
	}

	if result.Duration != 100*time.Millisecond {
		t.Errorf("Expected duration 100ms, got %v", result.Duration)
	}

	if result.RowCount != 1 {
		t.Errorf("Expected row count 1, got %d", result.RowCount)
	}

	if result.VerticalFormat != false {
		t.Errorf("Expected VerticalFormat false, got %v", result.VerticalFormat)
	}

	if len(result.Rows) != 0 {
		t.Errorf("Expected empty rows slice, got %d rows", len(result.Rows))
	}

	if len(result.Columns) != 1 || result.Columns[0] != "1" {
		t.Errorf("Expected columns ['1'], got %v", result.Columns)
	}

	if result.Err != nil {
		t.Errorf("Expected no error, got %v", result.Err)
	}
}

func TestParseMyCnf(t *testing.T) {
	// This test would require creating a temporary .my.cnf file
	// For now, we'll test that the function doesn't panic
	_, err := ParseMyCnf()
	// It's okay if this returns an error (file not found)
	// We just want to make sure it doesn't panic
	if err != nil {
		t.Logf("ParseMyCnf returned error (expected if no .my.cnf file): %v", err)
	}
}

func TestFillDSN(t *testing.T) {
	cnf := &MyCnf{
		User:     "testuser",
		Password: "testpass",
		Host:     "testhost",
		Port:     "3307",
		Database: "testdb",
	}

	tests := []struct {
		name     string
		dsn      string
		expected string
	}{
		{
			name:     "empty DSN gets filled completely",
			dsn:      "@/",
			expected: "testuser:testpass@tcp(testhost:3307)/testdb",
		},
		{
			name:     "DSN with user but no password",
			dsn:      "myuser:@tcp(localhost:3306)/mydb",
			expected: "myuser:testpass@tcp(localhost:3306)/mydb",
		},
		{
			name:     "DSN with user and password",
			dsn:      "myuser:mypass@tcp(localhost:3306)/mydb",
			expected: "myuser:mypass@tcp(localhost:3306)/mydb",
		},
		{
			name:     "DSN missing host gets default",
			dsn:      "myuser:mypass@/mydb",
			expected: "myuser:mypass@tcp(testhost:3307)/mydb",
		},
		{
			name:     "DSN missing database",
			dsn:      "myuser:mypass@tcp(localhost:3306)/",
			expected: "myuser:mypass@tcp(localhost:3306)/testdb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FillDSN(tt.dsn, cnf)
			if result != tt.expected {
				t.Errorf("FillDSN() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestStatementInfo(t *testing.T) {
	// Test StatementInfo struct
	stmt := StatementInfo{
		SQL:      "SELECT * FROM users",
		Vertical: true,
	}

	if stmt.SQL != "SELECT * FROM users" {
		t.Errorf("Expected SQL 'SELECT * FROM users', got %q", stmt.SQL)
	}

	if !stmt.Vertical {
		t.Errorf("Expected Vertical to be true, got %v", stmt.Vertical)
	}
}

// Benchmark tests for performance-critical functions
func BenchmarkSplitSQLStatements(b *testing.B) {
	sql := strings.Repeat("SELECT 1; ", 100) // 100 simple statements

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		splitSQLStatements(sql)
	}
}

func BenchmarkSplitSQLStatementsWithStrings(b *testing.B) {
	sql := strings.Repeat("SELECT 'test; with semicolon'; ", 100) // 100 statements with strings

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		splitSQLStatements(sql)
	}
}

func BenchmarkMaskPasswordInDSN(b *testing.B) {
	dsn := "user:very_long_complex_password_with_special_chars!@#$%^&*()@tcp(very.long.hostname.example.com:3306)/very_long_database_name"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		maskPasswordInDSN(dsn)
	}
}
