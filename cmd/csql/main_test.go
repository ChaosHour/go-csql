package main

import (
	"os"
	"strings"
	"testing"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config with instances and statements",
			config: Config{
				Instances:  "user:pass@tcp(host:3306)/db",
				Statements: "SELECT 1",
			},
			wantErr: false,
		},
		{
			name: "valid config with json file and statements",
			config: Config{
				JSONFile:   "servers.json",
				Statements: "SELECT 1",
			},
			wantErr: false,
		},
		{
			name: "invalid config - no instances or json",
			config: Config{
				Statements: "SELECT 1",
			},
			wantErr: true,
		},
		{
			name: "invalid config - no SQL source",
			config: Config{
				Instances: "user:pass@tcp(host:3306)/db",
			},
			wantErr: true,
		},
		{
			name: "valid config with stdin",
			config: Config{
				Instances: "user:pass@tcp(host:3306)/db",
				Stdin:     true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateDSN(t *testing.T) {
	tests := []struct {
		name    string
		dsn     string
		wantErr bool
	}{
		{
			name:    "valid DSN with tcp",
			dsn:     "user:pass@tcp(localhost:3306)/database",
			wantErr: false,
		},
		{
			name:    "valid DSN with unix socket",
			dsn:     "user:pass@unix(/var/run/mysqld/mysqld.sock)/database",
			wantErr: false,
		},
		{
			name:    "valid DSN minimal",
			dsn:     "user@tcp(localhost:3306)/database",
			wantErr: false,
		},
		{
			name:    "empty DSN",
			dsn:     "",
			wantErr: true,
		},
		{
			name:    "invalid DSN - no protocol",
			dsn:     "user:pass/database",
			wantErr: true,
		},
		{
			name:    "invalid DSN - unclosed protocol",
			dsn:     "user:pass@tcp(localhost:3306/database",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDSN(tt.dsn)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDSN() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateInstances(t *testing.T) {
	tests := []struct {
		name      string
		instances []string
		wantErr   bool
	}{
		{
			name:      "valid instances",
			instances: []string{"user:pass@tcp(host1:3306)/db1", "user:pass@tcp(host2:3306)/db2"},
			wantErr:   false,
		},
		{
			name:      "empty instances",
			instances: []string{},
			wantErr:   true,
		},
		{
			name:      "invalid instance in list",
			instances: []string{"user:pass@tcp(host1:3306)/db1", "invalid-dsn"},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInstances(tt.instances)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateInstances() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantPath string
		wantErr  bool
	}{
		{
			name:     "regular path",
			path:     "/etc/config",
			wantPath: "/etc/config",
			wantErr:  false,
		},
		{
			name:     "relative path",
			path:     "config.json",
			wantPath: "config.json",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, err := expandPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("expandPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.name != "home path" && gotPath != tt.wantPath {
				t.Errorf("expandPath() = %v, want %v", gotPath, tt.wantPath)
			}
		})
	}

	// Test home directory expansion separately
	t.Run("home path", func(t *testing.T) {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			t.Skip("Cannot get home directory")
		}
		
		gotPath, err := expandPath("~/config.json")
		if err != nil {
			t.Errorf("expandPath() error = %v", err)
			return
		}
		
		expectedPath := homeDir + "/config.json"
		if gotPath != expectedPath {
			t.Errorf("expandPath() = %v, want %v", gotPath, expectedPath)
		}
	})
}

func TestSanitizeDSN(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{
			name: "DSN with special characters in password",
			dsn:  "user:p@ss!w0rd@tcp(localhost:3306)/database",
			want: "user:p%40ss%21w0rd@tcp(localhost:3306)/database",
		},
		{
			name: "DSN without password",
			dsn:  "user@tcp(localhost:3306)/database",
			want: "user@tcp(localhost:3306)/database",
		},
		{
			name: "DSN with simple password",
			dsn:  "user:password@tcp(localhost:3306)/database",
			want: "user:password@tcp(localhost:3306)/database",
		},
		{
			name: "malformed DSN",
			dsn:  "invalid-dsn",
			want: "invalid-dsn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeDSN(tt.dsn); got != tt.want {
				t.Errorf("sanitizeDSN() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDsnHasHost(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want bool
	}{
		{
			name: "DSN with host",
			dsn:  "user:pass@tcp(localhost:3306)/database",
			want: true,
		},
		{
			name: "DSN with empty host",
			dsn:  "user:pass@tcp(:3306)/database",
			want: false,
		},
		{
			name: "DSN without tcp protocol",
			dsn:  "user:pass@/database",
			want: false,
		},
		{
			name: "malformed DSN",
			dsn:  "invalid",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dsnHasHost(tt.dsn); got != tt.want {
				t.Errorf("dsnHasHost() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseVerbosityFlags(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantVerbose  int
		wantFiltered []string
	}{
		{
			name:         "no verbosity flags",
			args:         []string{"--instances", "dsn", "--statements", "SELECT 1"},
			wantVerbose:  0,
			wantFiltered: []string{"--instances", "dsn", "--statements", "SELECT 1"},
		},
		{
			name:         "single -v flag",
			args:         []string{"-v", "--instances", "dsn"},
			wantVerbose:  1,
			wantFiltered: []string{"--instances", "dsn"},
		},
		{
			name:         "double -vv flag",
			args:         []string{"-vv", "--instances", "dsn"},
			wantVerbose:  2,
			wantFiltered: []string{"--instances", "dsn"},
		},
		{
			name:         "triple -vvv flag",
			args:         []string{"-vvv", "--instances", "dsn"},
			wantVerbose:  3,
			wantFiltered: []string{"--instances", "dsn"},
		},
		{
			name:         "-v=2 format",
			args:         []string{"-v=2", "--instances", "dsn"},
			wantVerbose:  2,
			wantFiltered: []string{"--instances", "dsn"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Temporarily replace os.Args
			originalArgs := os.Args
			os.Args = append([]string{"go-csql"}, tt.args...)
			defer func() { os.Args = originalArgs }()

			gotVerbose, gotFiltered := parseVerbosityFlags()
			if gotVerbose != tt.wantVerbose {
				t.Errorf("parseVerbosityFlags() verbose = %v, want %v", gotVerbose, tt.wantVerbose)
			}
			if !stringSliceEqual(gotFiltered, tt.wantFiltered) {
				t.Errorf("parseVerbosityFlags() filtered = %v, want %v", gotFiltered, tt.wantFiltered)
			}
		})
	}
}

// Helper function to compare string slices
func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestStripJSONComments(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "JSON with comments",
			input: `# This is a comment
{
  "user": "testuser",
  # Another comment
  "password": "testpass"
}`,
			want: `{
  "user": "testuser",
  "password": "testpass"
}`,
		},
		{
			name: "JSON without comments",
			input: `{
  "user": "testuser",
  "password": "testpass"
}`,
			want: `{
  "user": "testuser",
  "password": "testpass"
}`,
		},
		{
			name: "password starting with #",
			input: `{
  "user": "testuser",
  "password": "#complex!password"
}`,
			want: `{
  "user": "testuser",
  "password": "#complex!password"
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(stripJSONComments([]byte(tt.input)))
			// Normalize whitespace for comparison
			gotNorm := strings.TrimSpace(strings.ReplaceAll(got, "\n\n", "\n"))
			wantNorm := strings.TrimSpace(strings.ReplaceAll(tt.want, "\n\n", "\n"))
			
			if gotNorm != wantNorm {
				t.Errorf("stripJSONComments() = %q, want %q", gotNorm, wantNorm)
			}
		})
	}
}
