package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ChaosHour/go-csql/pkg/db"
	"github.com/fatih/color"
)

// Define a list of colors to cycle through for different instances
var instanceColors = []*color.Color{
	color.New(color.FgCyan),
	color.New(color.FgGreen),
	color.New(color.FgYellow),
	color.New(color.FgBlue),
	color.New(color.FgMagenta),
	color.New(color.FgRed),
}

// Config holds all configuration for the CLI application
type Config struct {
	Instances   string
	Statements  string
	File        string
	JSONFile    string
	SQLFile     string
	Stdin       bool
	Concurrent  bool
	TableFormat bool
	Verbose     int
}

// Server represents a database server configuration
type Server struct {
	DSN      string `json:"dsn,omitempty"`      // Traditional DSN format
	User     string `json:"user,omitempty"`     // Separate user field
	Password string `json:"password,omitempty"` // Separate password field
	Host     string `json:"host,omitempty"`     // Separate host field
	Port     string `json:"port,omitempty"`     // Separate port field
	Database string `json:"database,omitempty"` // Separate database field
}

// BuildDSN constructs a proper DSN from Server fields, handling complex passwords
func (s *Server) BuildDSN() string {
	if s.DSN != "" {
		return s.DSN // Use DSN if provided
	}

	// Build DSN from individual components
	var dsn strings.Builder

	if s.User != "" {
		dsn.WriteString(s.User)
		if s.Password != "" {
			dsn.WriteString(":")
			// URL encode the password to handle special characters
			dsn.WriteString(url.QueryEscape(s.Password))
		}
		dsn.WriteString("@")
	}

	dsn.WriteString("tcp(")
	if s.Host != "" {
		dsn.WriteString(s.Host)
	} else {
		dsn.WriteString("localhost")
	}

	if s.Port != "" {
		dsn.WriteString(":")
		dsn.WriteString(s.Port)
	} else {
		dsn.WriteString(":3306")
	}
	dsn.WriteString(")")

	if s.Database != "" {
		dsn.WriteString("/")
		dsn.WriteString(s.Database)
	}

	return dsn.String()
}

// parseVerbosityFlags handles -v, -vv, -vvv style flags manually
func parseVerbosityFlags() (int, []string) {
	var verbose int
	var filteredArgs []string

	for _, arg := range os.Args[1:] {
		if arg == "-v" {
			verbose = 1
		} else if arg == "-vv" {
			verbose = 2
		} else if arg == "-vvv" {
			verbose = 3
		} else if strings.HasPrefix(arg, "-v=") {
			// Handle -v=1, -v=2, -v=3 format
			if val := strings.TrimPrefix(arg, "-v="); val != "" {
				switch val {
				case "1":
					verbose = 1
				case "2":
					verbose = 2
				case "3":
					verbose = 3
				}
			}
		} else {
			// Keep non-verbosity flags for standard parsing
			filteredArgs = append(filteredArgs, arg)
		}
	}
	return verbose, filteredArgs
}

// LoadFromFlags parses command line flags and populates the Config
func (c *Config) LoadFromFlags() error {
	// Handle verbosity flags first
	verbose, filteredArgs := parseVerbosityFlags()
	c.Verbose = verbose

	// Temporarily replace os.Args for flag parsing
	originalArgs := os.Args
	os.Args = append([]string{os.Args[0]}, filteredArgs...)
	defer func() { os.Args = originalArgs }()

	// CLI flags
	instances := flag.String("instances", "", "Comma-separated list of MySQL instance connection strings (user:password@tcp(host:port)/dbname)")
	statements := flag.String("statements", "", "Semicolon-separated list of SQL statements to execute")
	file := flag.String("file", "", "Path to a file containing SQL statements (overrides --statements)")
	jsonFile := flag.String("json", "", "Path to a JSON file with server and schema information (overrides --instances)")
	sqlFile := flag.String("sqlfile", "", "Path to a .txt file with SQL statements (overrides --statements and --file)")
	stdin := flag.Bool("stdin", false, "Read SQL statements from standard input (pipe support)")
	concurrent := flag.Bool("concurrent", true, "Run queries against instances concurrently")
	tableFormat := flag.Bool("table", false, "Format tabular output with borders")

	// Parse flags
	flag.Parse()

	// Populate config
	c.Instances = *instances
	c.Statements = *statements
	c.File = *file
	c.JSONFile = *jsonFile
	c.SQLFile = *sqlFile
	c.Stdin = *stdin
	c.Concurrent = *concurrent
	c.TableFormat = *tableFormat

	return nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Instances == "" && c.JSONFile == "" {
		return fmt.Errorf("--instances or --json is required")
	}

	sqlSourceCount := 0
	if c.Stdin {
		sqlSourceCount++
	}
	if c.SQLFile != "" {
		sqlSourceCount++
	}
	if c.File != "" {
		sqlSourceCount++
	}
	if c.Statements != "" {
		sqlSourceCount++
	}

	if sqlSourceCount == 0 {
		return fmt.Errorf("must provide --stdin, --sqlfile, --file, or --statements")
	}

	return nil
}

// validateDSN validates a MySQL DSN format
func validateDSN(dsn string) error {
	if dsn == "" {
		return fmt.Errorf("DSN cannot be empty")
	}

	// Basic DSN format validation
	// Expected format: [user[:password]@][protocol[(address)]]/dbname[?param1=value1&...]

	// Check for protocol part
	if !strings.Contains(dsn, "@tcp(") && !strings.Contains(dsn, "@unix(") && !strings.Contains(dsn, "@") {
		return fmt.Errorf("invalid DSN format: missing protocol or @ symbol")
	}

	// If it contains @tcp( or @unix(, validate the structure
	if strings.Contains(dsn, "@tcp(") || strings.Contains(dsn, "@unix(") {
		protocolStart := strings.Index(dsn, "@")
		if protocolStart == -1 {
			return fmt.Errorf("invalid DSN format: malformed protocol section")
		}

		protocolEnd := strings.Index(dsn[protocolStart:], ")")
		if protocolEnd == -1 {
			return fmt.Errorf("invalid DSN format: unclosed protocol section")
		}
	}

	return nil
}

// validateInstances validates a list of DSN strings
func validateInstances(instances []string) error {
	if len(instances) == 0 {
		return fmt.Errorf("no instances provided")
	}

	for i, dsn := range instances {
		if err := validateDSN(dsn); err != nil {
			return fmt.Errorf("invalid DSN at index %d: %w", i, err)
		}
	}

	return nil
}

// expandPath expands ~ to home directory in file paths
func expandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(homeDir, path[2:]), nil
	}
	return path, nil
}

// stripJSONComments removes comment lines from JSON content while preserving strings that may contain #
func stripJSONComments(content []byte) []byte {
	lines := strings.Split(string(content), "\n")
	var cleanLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Only skip lines that are pure comments (start with # and are not inside JSON strings)
		// A line is a comment if it starts with # and doesn't contain JSON syntax like quotes, braces, etc.
		if strings.HasPrefix(trimmed, "#") && !containsJSONSyntax(trimmed) {
			continue // Skip comment lines
		}
		if trimmed != "" || len(cleanLines) > 0 { // Keep non-empty lines or preserve structure
			cleanLines = append(cleanLines, line)
		}
	}

	return []byte(strings.Join(cleanLines, "\n"))
}

// containsJSONSyntax checks if a line contains JSON syntax characters that would indicate it's not a pure comment
func containsJSONSyntax(line string) bool {
	// Look for JSON syntax that would indicate this is not a pure comment
	jsonChars := []string{`"`, `'`, `{`, `}`, `[`, `]`, `:`, `,`}
	for _, char := range jsonChars {
		if strings.Contains(line, char) {
			return true
		}
	}
	return false
}

// LoadInstances loads and processes database instances from config
func (c *Config) LoadInstances() ([]string, error) {
	var myCnf *db.MyCnf
	myCnf, _ = db.ParseMyCnf() // ignore error if file doesn't exist

	var instanceList []string
	var err error

	if c.JSONFile != "" {
		instanceList, err = c.loadInstancesFromJSON(myCnf)
	} else {
		instanceList, err = c.loadInstancesFromFlag(myCnf)
	}

	if err != nil {
		return nil, err
	}

	// Validate all instances
	if err := validateInstances(instanceList); err != nil {
		return nil, fmt.Errorf("instance validation failed: %w", err)
	}

	return instanceList, nil
}

// loadInstancesFromJSON loads instances from JSON file
func (c *Config) loadInstancesFromJSON(myCnf *db.MyCnf) ([]string, error) {
	var instanceList []string

	// Expand ~ to home directory
	expandedPath, err := expandPath(c.JSONFile)
	if err != nil {
		return nil, fmt.Errorf("failed to expand JSON file path: %w", err)
	}

	// JSON file format supports both DSN strings and individual components
	content, err := os.ReadFile(expandedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read JSON file: %w", err)
	}

	// Strip comments from JSON content
	cleanContent := stripJSONComments(content)

	var servers []Server
	err = json.Unmarshal(cleanContent, &servers)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	for _, s := range servers {
		dsnToUse := s.BuildDSN() // Build DSN with proper password encoding
		if myCnf != nil {
			// Apply .my.cnf credentials respecting existing host info
			if !dsnHasHost(dsnToUse) {
				dsnToUse = db.FillDSN(dsnToUse, myCnf)
			} else {
				// Create a temporary cnf without host to fill other details
				tempCnf := *myCnf
				tempCnf.Host = "" // Don't override host from .my.cnf
				dsnToUse = db.FillDSN(dsnToUse, &tempCnf)
			}
		}
		instanceList = append(instanceList, dsnToUse)
	}

	return instanceList, nil
}

// loadInstancesFromFlag loads instances from command line flag
func (c *Config) loadInstancesFromFlag(myCnf *db.MyCnf) ([]string, error) {
	var instanceList []string
	rawInstances := strings.Split(c.Instances, ",")

	for _, dsn := range rawInstances {
		dsnToUse := strings.TrimSpace(dsn)
		if dsnToUse == "" {
			continue
		}
		dsnToUse = sanitizeDSN(dsnToUse) // Sanitize complex passwords
		if myCnf != nil {
			// Apply .my.cnf credentials respecting existing host info
			if !dsnHasHost(dsnToUse) {
				dsnToUse = db.FillDSN(dsnToUse, myCnf)
			} else {
				// Create a temporary cnf without host to fill other details
				tempCnf := *myCnf
				tempCnf.Host = "" // Don't override host from .my.cnf
				dsnToUse = db.FillDSN(dsnToUse, &tempCnf)
			}
		}
		instanceList = append(instanceList, dsnToUse)
	}

	return instanceList, nil
}

// LoadStatements loads SQL statements from various sources
func (c *Config) LoadStatements() (string, error) {
	if c.Stdin {
		return c.loadStatementsFromStdin()
	}
	if c.SQLFile != "" {
		return c.loadStatementsFromFile(c.SQLFile)
	}
	if c.File != "" {
		return c.loadStatementsFromFile(c.File)
	}
	if c.Statements != "" {
		return c.Statements, nil
	}
	return "", fmt.Errorf("no SQL statements provided")
}

// loadStatementsFromStdin reads SQL statements from standard input
func (c *Config) loadStatementsFromStdin() (string, error) {
	scanner := bufio.NewScanner(os.Stdin)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading from stdin: %w", err)
	}
	return strings.Join(lines, "\n"), nil
}

// loadStatementsFromFile reads SQL statements from a file
func (c *Config) loadStatementsFromFile(filename string) (string, error) {
	// Expand ~ to home directory
	expandedPath, err := expandPath(filename)
	if err != nil {
		return "", fmt.Errorf("failed to expand file path: %w", err)
	}
	content, err := os.ReadFile(expandedPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	return string(content), nil
}

// run contains the main application logic
func run() error {
	config := &Config{}

	// Load configuration from flags
	if err := config.LoadFromFlags(); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return err
	}

	// Load instances
	instanceList, err := config.LoadInstances()
	if err != nil {
		return fmt.Errorf("failed to load instances: %w", err)
	}

	if len(instanceList) == 0 {
		return fmt.Errorf("no valid instances found after processing flags and files")
	}

	// Load SQL statements
	sqls, err := config.LoadStatements()
	if err != nil {
		return fmt.Errorf("failed to load statements: %w", err)
	}

	// Execute queries
	return executeQueries(config, instanceList, sqls)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// executeQueries handles the execution of SQL queries against instances
func executeQueries(config *Config, instanceList []string, sqls string) error {
	// --- Assign colors to instances ---
	instanceColorMap := make(map[string]*color.Color)
	for i, instanceDSN := range instanceList {
		instanceColorMap[instanceDSN] = instanceColors[i%len(instanceColors)]
	}

	// --- Execute Concurrently or Sequentially ---
	fmt.Printf("Executing statements on %d instance(s) (concurrent: %t)...\n", len(instanceList), config.Concurrent)

	if config.Concurrent {
		// --- Execute Concurrently ---
		type instanceResult struct {
			instance string
			results  []db.QueryResult
			err      error
		}

		var wg sync.WaitGroup
		resultsChan := make(chan instanceResult, len(instanceList)) // Buffered channel

		for _, instanceDSN := range instanceList {
			wg.Add(1)
			go func(dsn string) {
				defer func() {
					if r := recover(); r != nil {
						resultsChan <- instanceResult{
							instance: dsn,
							err:      fmt.Errorf("panic in goroutine: %v", r),
						}
					}
					wg.Done()
				}()

				// Run SQL for this specific instance
				instanceResults := db.RunSQLOnInstanceWithVerbosity(dsn, sqls, config.Verbose)
				resultsChan <- instanceResult{
					instance: dsn,
					results:  instanceResults,
				}
			}(instanceDSN) // Pass instanceDSN to the goroutine
		}

		// Wait for all goroutines to complete
		wg.Wait()
		close(resultsChan)

		// Collect all results and maintain order
		allResults := make(map[string][]db.QueryResult)
		var errors []error

		for result := range resultsChan {
			if result.err != nil {
				errors = append(errors, fmt.Errorf("instance %s: %w", result.instance, result.err))
			} else {
				allResults[result.instance] = result.results
			}
		}

		// Print any goroutine errors
		for _, err := range errors {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}

		// Print results in the original instance order
		for _, instanceDSN := range instanceList {
			if results, exists := allResults[instanceDSN]; exists {
				instanceColor := instanceColorMap[instanceDSN]
				for _, res := range results {
					db.PrintResultWithVerbosity(res, instanceColor, config.TableFormat, config.Verbose)
					fmt.Println("---")
				}
			}
		}
	} else {
		// --- Execute Sequentially ---
		for _, instanceDSN := range instanceList {
			instanceColor := instanceColorMap[instanceDSN] // Get color for this instance
			instanceResults := db.RunSQLOnInstanceWithVerbosity(instanceDSN, sqls, config.Verbose)
			for _, res := range instanceResults {
				db.PrintResultWithVerbosity(res, instanceColor, config.TableFormat, config.Verbose)
				fmt.Println("---") // Separator between results
			}
		}
	}

	fmt.Println("All executions complete.")
	return nil
}

// sanitizeDSN safely handles complex passwords by URL encoding them
func sanitizeDSN(dsn string) string {
	// Parse DSN format: user:password@tcp(host:port)/database
	atIndex := strings.LastIndex(dsn, "@")
	if atIndex == -1 {
		return dsn // Return as-is if not in expected format
	}

	userPass := dsn[:atIndex]
	rest := dsn[atIndex:]

	// Split user:password
	colonIndex := strings.Index(userPass, ":")
	if colonIndex == -1 {
		return dsn // Return as-is if no password
	}

	user := userPass[:colonIndex]
	password := userPass[colonIndex+1:]

	// URL encode the password to handle special characters
	encodedPassword := url.QueryEscape(password)

	return user + ":" + encodedPassword + rest
}

// dsnHasHost returns true if the DSN contains a host in the tcp(...) section
func dsnHasHost(dsn string) bool {
	// Find the protocol part like @tcp( or @unix(
	protoIdx := strings.Index(dsn, "@")
	if protoIdx == -1 {
		return false // Malformed or simple DSN without protocol/host part
	}

	// Look specifically for tcp(
	tcpIdx := strings.Index(dsn[protoIdx:], "@tcp(")
	if tcpIdx == -1 {
		return false // Not using tcp protocol specification
	}

	// Adjust index relative to the start of the string
	startHostIdx := protoIdx + tcpIdx + len("@tcp(")

	// Find the closing parenthesis
	endHostIdx := strings.Index(dsn[startHostIdx:], ")")
	if endHostIdx == -1 {
		return false // Malformed DSN
	}

	// Extract host:port part
	hostPort := dsn[startHostIdx : startHostIdx+endHostIdx]

	// Check if host part is non-empty (before the colon if present)
	hostParts := strings.SplitN(hostPort, ":", 2)
	return len(hostParts) > 0 && strings.TrimSpace(hostParts[0]) != ""
}
