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
	"sync" // Import sync package

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

func main() {
	// CLI flags
	instances := flag.String("instances", "", "Comma-separated list of MySQL instance connection strings (user:password@tcp(host:port)/dbname)")
	statements := flag.String("statements", "", "Semicolon-separated list of SQL statements to execute")
	file := flag.String("file", "", "Path to a file containing SQL statements (overrides --statements)")
	jsonFile := flag.String("json", "", "Path to a JSON file with server and schema information (overrides --instances)")
	sqlFile := flag.String("sqlfile", "", "Path to a .txt file with SQL statements (overrides --statements and --file)")
	stdin := flag.Bool("stdin", false, "Read SQL statements from standard input (pipe support)")
	concurrent := flag.Bool("concurrent", true, "Run queries against instances concurrently")
	tableFormat := flag.Bool("table", false, "Format tabular output with borders")
	verbose := flag.Int("v", 0, "Verbosity level (1-3): -v, -vv, -vvv for increasing verbosity")
	flag.Parse()

	// Handle -v, -vv, -vvv style flags
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-v") && !strings.Contains(arg, "=") {
			*verbose = len(arg) - 1 // -v=1, -vv=2, -vvv=3
			break
		}
	}

	if *instances == "" && *jsonFile == "" {
		fmt.Println("Error: --instances or --json is required")
		os.Exit(1)
	}

	// --- Load Instances ---
	var instanceList []string
	var myCnf *db.MyCnf
	myCnf, _ = db.ParseMyCnf() // ignore error if file doesn't exist

	if *jsonFile != "" {
		// Expand ~ to home directory
		expandedPath, err := expandPath(*jsonFile)
		if err != nil {
			fmt.Printf("Failed to expand JSON file path: %v\n", err)
			os.Exit(1)
		}

		// JSON file format supports both DSN strings and individual components
		content, err := os.ReadFile(expandedPath)
		if err != nil {
			fmt.Printf("Failed to read JSON file: %v\n", err)
			os.Exit(1)
		}

		// Strip comments from JSON content
		cleanContent := stripJSONComments(content)

		var servers []Server
		err = json.Unmarshal(cleanContent, &servers)
		if err != nil {
			fmt.Printf("Failed to parse JSON: %v\n", err)
			os.Exit(1)
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
	} else {
		rawInstances := strings.Split(*instances, ",")
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
	}

	if len(instanceList) == 0 {
		fmt.Println("Error: No valid instances found after processing flags and files.")
		os.Exit(1)
	}

	// --- Load SQL Statements ---
	var sqls string
	if *stdin {
		// Read from standard input
		scanner := bufio.NewScanner(os.Stdin)
		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			fmt.Printf("Error reading from stdin: %v\n", err)
			os.Exit(1)
		}
		sqls = strings.Join(lines, "\n")
	} else if *sqlFile != "" {
		// Expand ~ to home directory
		expandedPath, err := expandPath(*sqlFile)
		if err != nil {
			fmt.Printf("Failed to expand SQL file path: %v\n", err)
			os.Exit(1)
		}
		content, err := os.ReadFile(expandedPath)
		if err != nil {
			fmt.Printf("Failed to read SQL file: %v\n", err)
			os.Exit(1)
		}
		sqls = string(content)
	} else if *file != "" {
		// Expand ~ to home directory
		expandedPath, err := expandPath(*file)
		if err != nil {
			fmt.Printf("Failed to expand file path: %v\n", err)
			os.Exit(1)
		}
		content, err := os.ReadFile(expandedPath)
		if err != nil {
			fmt.Printf("Failed to read file: %v\n", err)
			os.Exit(1)
		}
		sqls = string(content)
	} else if *statements != "" {
		sqls = *statements
	} else {
		fmt.Println("Error: must provide --stdin, --sqlfile, --file, or --statements")
		os.Exit(1)
	}

	// --- Assign colors to instances ---
	instanceColorMap := make(map[string]*color.Color)
	for i, instanceDSN := range instanceList {
		instanceColorMap[instanceDSN] = instanceColors[i%len(instanceColors)]
	}

	// --- Execute Concurrently or Sequentially ---
	fmt.Printf("Executing statements on %d instance(s) (concurrent: %t)...\n", len(instanceList), *concurrent)

	if *concurrent {
		// --- Execute Concurrently ---
		var wg sync.WaitGroup
		resultsChan := make(chan db.QueryResult) // Channel to receive results

		for _, instanceDSN := range instanceList {
			wg.Add(1)
			go func(dsn string) {
				defer wg.Done()
				// Run SQL for this specific instance
				instanceResults := db.RunSQLOnInstance(dsn, sqls)
				// Send each result (or error result) to the channel
				for _, res := range instanceResults {
					resultsChan <- res
				}
			}(instanceDSN) // Pass instanceDSN to the goroutine
		}

		// Goroutine to close the channel once all workers are done
		go func() {
			wg.Wait()
			close(resultsChan)
		}()

		// Process results as they come in from the channel
		for res := range resultsChan {
			instanceColor := instanceColorMap[res.Instance]  // Get color for this instance
			db.PrintResult(res, instanceColor, *tableFormat) // Pass tableFormat flag
			fmt.Println("---")                               // Separator between results
		}
	} else {
		// --- Execute Sequentially ---
		for _, instanceDSN := range instanceList {
			instanceColor := instanceColorMap[instanceDSN] // Get color for this instance
			instanceResults := db.RunSQLOnInstance(instanceDSN, sqls)
			for _, res := range instanceResults {
				db.PrintResult(res, instanceColor, *tableFormat) // Pass tableFormat flag
				fmt.Println("---")                               // Separator between results
			}
		}
	}

	fmt.Println("All executions complete.")
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
