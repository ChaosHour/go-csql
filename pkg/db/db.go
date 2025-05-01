package db

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"

	// Needed for robust DSN parsing
	"github.com/fatih/color"
	_ "github.com/go-sql-driver/mysql"
	"github.com/olekukonko/tablewriter" // Import tablewriter
)

// StatementInfo holds the SQL and whether vertical formatting is requested
type StatementInfo struct {
	SQL      string
	Vertical bool
}

type QueryResult struct {
	Instance       string
	Statement      string // The original statement including \G if used
	Rows           [][]interface{}
	Columns        []string
	Err            error
	VerticalFormat bool // Flag to indicate vertical output
}

// RunSQLOnInstance connects to a single instance and executes all SQL statements.
func RunSQLOnInstance(instanceDSN string, sqls string) []QueryResult {
	statementList := splitSQLStatements(sqls) // Now returns []StatementInfo
	results := []QueryResult{}

	// Trim space from instance DSN just in case
	instanceDSN = strings.TrimSpace(instanceDSN)

	db, err := sql.Open("mysql", instanceDSN)
	if err != nil {
		// Return a single error result for the whole instance if connection fails
		results = append(results, QueryResult{Instance: instanceDSN, Err: fmt.Errorf("failed to open connection: %w", err)})
		return results
	}
	defer db.Close()

	// Ping to verify connection early
	err = db.Ping()
	if err != nil {
		results = append(results, QueryResult{Instance: instanceDSN, Err: fmt.Errorf("failed to ping database: %w", err)})
		return results
	}

	for _, stmtInfo := range statementList {
		// Use stmtInfo.SQL (without \G) for query execution
		// Use stmtInfo.SQL (original, potentially with \G) for reporting in QueryResult
		stmtToExecute := stmtInfo.SQL
		originalStmt := stmtInfo.SQL // Store original for reporting
		if stmtInfo.Vertical {
			originalStmt += "\\G" // Add back for display if needed, or just use the flag
		}

		rows, err := db.Query(stmtToExecute)
		if err != nil {
			results = append(results, QueryResult{Instance: instanceDSN, Statement: originalStmt, Err: fmt.Errorf("query error: %w", err), VerticalFormat: stmtInfo.Vertical})
			continue // Move to the next statement
		}

		// Process rows even if there's an error getting columns later
		cols, colErr := rows.Columns()
		var allRows [][]interface{}
		var scanErr error

		if colErr == nil {
			for rows.Next() {
				vals := make([]interface{}, len(cols))
				scanArgs := make([]interface{}, len(cols))
				for i := range vals {
					scanArgs[i] = &vals[i]
				}
				scanErr = rows.Scan(scanArgs...)
				if scanErr != nil {
					// Log scan error but continue processing other rows/statements
					fmt.Fprintf(os.Stderr, "[%s] %s - Row scan error: %v\n", instanceDSN, stmtToExecute, scanErr)
					// Store the first scan error encountered for this statement result
					if err == nil { // Only capture the first error
						err = fmt.Errorf("row scan error: %w", scanErr)
					}
					continue // Skip this row
				}
				// Copy values as Scan reuses the buffer
				rowCopy := make([]interface{}, len(vals))
				for i, v := range vals {
					// Handle potential nil values from DB
					if b, ok := v.([]byte); ok {
						rowCopy[i] = string(b) // Convert bytes to string for better display
					} else {
						rowCopy[i] = v
					}
				}
				allRows = append(allRows, rowCopy)
			}
		} else {
			// If getting columns failed, record that error
			err = fmt.Errorf("failed to get columns: %w", colErr)
		}

		// Check for errors encountered during row iteration
		if rows.Err() != nil {
			if err == nil { // Prioritize earlier errors
				err = fmt.Errorf("rows iteration error: %w", rows.Err())
			}
		}

		// Make sure to pass the Vertical flag when creating the result
		results = append(results, QueryResult{
			Instance:       instanceDSN,
			Statement:      originalStmt, // Report the statement as entered
			Rows:           allRows,
			Columns:        cols,
			Err:            err, // Includes potential scan/column errors
			VerticalFormat: stmtInfo.Vertical,
		})
		rows.Close() // Close rows as soon as possible
	}

	return results
}

// splitSQLStatements splits SQL string and detects \G
func splitSQLStatements(sqls string) []StatementInfo {
	var statements []StatementInfo
	// Basic split, doesn't handle semicolons in strings/comments well
	rawStatements := strings.Split(sqls, ";")
	for _, s := range rawStatements {
		trimmed := strings.TrimSpace(s)
		if trimmed == "" {
			continue
		}

		info := StatementInfo{SQL: trimmed, Vertical: false}
		if strings.HasSuffix(trimmed, "\\G") {
			info.Vertical = true
			// Remove \G for execution
			info.SQL = strings.TrimSpace(trimmed[:len(trimmed)-2])
		}
		// Only add if the SQL part is not empty after removing \G
		if info.SQL != "" {
			statements = append(statements, info)
		}
	}
	return statements
}

// maskPasswordInDSN takes a DSN string and returns a version with the password masked.
func maskPasswordInDSN(dsn string) string {
	// Attempt to parse the DSN as a URL-like string to handle various formats
	// MySQL DSN format: [user[:password]@][protocol[(address)]]/dbname[?param1=value1&...]
	// We need to handle the part before the first '/' which isn't standard URL parsing.

	// Find the '@' symbol separating user/pass from the host info
	atIndex := strings.Index(dsn, "@")
	if atIndex == -1 {
		return dsn // No user/password info found
	}

	userInfo := dsn[:atIndex]
	hostInfo := dsn[atIndex+1:]

	// Split user info into user and password
	userPass := strings.SplitN(userInfo, ":", 2)
	user := userPass[0]

	if len(userPass) == 2 {
		// Password exists, mask it
		return user + ":****@" + hostInfo
	}

	// No password, return as is (user@host...)
	return dsn
}

// PrintResult prints the query result, handling vertical and table formats.
func PrintResult(res QueryResult, instanceColor *color.Color, useTableFormat bool) { // Added useTableFormat param
	maskedDSN := maskPasswordInDSN(res.Instance)                     // Mask the password
	instanceStr := instanceColor.SprintFunc()("[" + maskedDSN + "]") // Use masked DSN
	if res.Err != nil {
		errorColor := color.New(color.FgRed).SprintFunc()
		fmt.Printf("%s %s %s: %v\n", instanceStr, errorColor("ERROR"), res.Statement, res.Err)
		return
	}
	fmt.Printf("%s %s\n", instanceStr, res.Statement)

	if res.VerticalFormat {
		// --- Vertical Output ---
		// ... (vertical logic remains the same) ...
		if len(res.Rows) == 0 {
			fmt.Println("Empty set.")
			return
		}
		rowSeparator := strings.Repeat("*", 20)
		maxColWidth := 0
		for _, colName := range res.Columns {
			if len(colName) > maxColWidth {
				maxColWidth = len(colName)
			}
		}
		for i, row := range res.Rows {
			fmt.Printf("%s %d. row %s\n", rowSeparator, i+1, rowSeparator)
			for j, colName := range res.Columns {
				valStr := "NULL"
				if j < len(row) && row[j] != nil {
					if b, ok := row[j].([]byte); ok {
						valStr = string(b)
					} else {
						valStr = fmt.Sprintf("%v", row[j])
					}
				}
				fmt.Printf("%*s: %s\n", maxColWidth, colName, valStr)
			}
		}
	} else if useTableFormat {
		// --- Table Writer Output ---
		if len(res.Columns) == 0 {
			fmt.Println("Statement executed successfully, no columns returned.")
			return
		}
		if len(res.Rows) == 0 {
			fmt.Println("Empty set.")
			return
		}

		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader(res.Columns)
		// Settings for MySQL client-like borders and wrapping:
		table.SetAutoWrapText(true) // Enable text wrapping
		table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
		table.SetAlignment(tablewriter.ALIGN_LEFT)
		table.SetHeaderLine(true) // Use RowSeparator for header line
		table.SetBorder(true)     // Enable overall border (+ corners)
		table.SetRowLine(false)   // Disable lines between data rows

		// Use MySQL client style border characters (+, -, |)
		table.SetCenterSeparator("+") // Character for intersections
		table.SetColumnSeparator("|") // Character for vertical lines
		table.SetRowSeparator("-")    // Character for horizontal lines
		// Ensure SetBorders is not used, as it can override separators

		// Convert rows to [][]string for tablewriter
		data := make([][]string, len(res.Rows))
		for i, row := range res.Rows {
			data[i] = make([]string, len(row))
			for j, v := range row {
				if b, ok := v.([]byte); ok {
					data[i][j] = string(b)
				} else if v == nil {
					data[i][j] = "NULL"
				} else {
					data[i][j] = fmt.Sprintf("%v", v)
				}
			}
		}
		table.AppendBulk(data)
		table.Render()

	} else {
		// --- Standard Tabular Output (Default) ---
		// ... (existing default tabular logic remains the same) ...
		if len(res.Columns) == 0 {
			fmt.Println("Statement executed successfully, no columns returned.")
			return
		}
		bold := color.New(color.Bold).SprintFunc()
		fmt.Println(bold(strings.Join(res.Columns, "\t")))
		if len(res.Rows) == 0 {
			fmt.Println("Empty set.")
			return
		}
		for _, row := range res.Rows {
			rowStrings := make([]string, len(row))
			for i, v := range row {
				if b, ok := v.([]byte); ok {
					rowStrings[i] = string(b)
				} else if v == nil {
					rowStrings[i] = "NULL"
				} else {
					rowStrings[i] = fmt.Sprintf("%v", v)
				}
			}
			fmt.Println(strings.Join(rowStrings, "\t"))
		}
	}
}

// MyCnf holds credentials from ~/.my.cnf
type MyCnf struct {
	User     string
	Password string
	Host     string
	Port     string
	Database string
}

// ParseMyCnf parses ~/.my.cnf for credentials
func ParseMyCnf() (*MyCnf, error) {
	usr, err := user.Current()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(usr.HomeDir, ".my.cnf")
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cnf := &MyCnf{}
	scanner := bufio.NewScanner(f)
	keyVal := regexp.MustCompile(`^([a-zA-Z_]+)\s*=\s*(.*)$`)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		m := keyVal.FindStringSubmatch(line)
		if len(m) == 3 {
			switch strings.ToLower(m[1]) {
			case "user":
				cnf.User = m[2]
			case "password":
				cnf.Password = m[2]
			case "host":
				cnf.Host = m[2]
			case "port":
				cnf.Port = m[2]
			case "database":
				cnf.Database = m[2]
			}
		}
	}
	return cnf, nil
}

// FillDSN fills missing DSN parts from MyCnf
func FillDSN(dsn string, cnf *MyCnf) string {
	// Only fill if DSN is missing user/password/host/port/db
	user, pass, netloc, db := "", "", "", ""
	// Parse DSN: user:pass@tcp(host:port)/db
	parts := strings.SplitN(dsn, "@", 2)
	if len(parts) == 2 {
		up := strings.SplitN(parts[0], ":", 2)
		if len(up) > 0 && up[0] != "" {
			user = up[0]
		}
		if len(up) == 2 && up[1] != "" {
			pass = up[1]
		}
		netdb := strings.SplitN(parts[1], "/", 2)
		if len(netdb) > 0 && netdb[0] != "" {
			netloc = netdb[0]
		}
		if len(netdb) == 2 && netdb[1] != "" {
			db = netdb[1]
		}
	}
	if user == "" && cnf.User != "" {
		user = cnf.User
	}
	if pass == "" && cnf.Password != "" {
		pass = cnf.Password
	}
	if netloc == "" {
		host := "localhost"
		if cnf.Host != "" {
			host = cnf.Host
		}
		port := "3306"
		if cnf.Port != "" {
			port = cnf.Port
		}
		netloc = "tcp(" + host + ":" + port + ")"
	}
	if db == "" && cnf.Database != "" {
		db = cnf.Database
	}
	return user + ":" + pass + "@" + netloc + "/" + db
}
