package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
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

func main() {
	// CLI flags
	instances := flag.String("instances", "", "Comma-separated list of MySQL instance connection strings (user:password@tcp(host:port)/dbname)")
	statements := flag.String("statements", "", "Semicolon-separated list of SQL statements to execute")
	file := flag.String("file", "", "Path to a file containing SQL statements (overrides --statements)")
	jsonFile := flag.String("json", "", "Path to a JSON file with server and schema information (overrides --instances)")
	sqlFile := flag.String("sqlfile", "", "Path to a .txt file with SQL statements (overrides --statements and --file)")
	concurrent := flag.Bool("concurrent", true, "Run queries against instances concurrently")
	tableFormat := flag.Bool("table", false, "Format tabular output with borders") // New flag
	flag.Parse()

	if *instances == "" && *jsonFile == "" {
		fmt.Println("Error: --instances or --json is required")
		os.Exit(1)
	}

	// --- Load Instances ---
	var instanceList []string
	var myCnf *db.MyCnf
	myCnf, _ = db.ParseMyCnf() // ignore error if file doesn't exist

	if *jsonFile != "" {
		// JSON file format: [{"dsn": "user:password@tcp(host:port)/dbname"}, ...]
		content, err := os.ReadFile(*jsonFile)
		if err != nil {
			fmt.Printf("Failed to read JSON file: %v\n", err)
			os.Exit(1)
		}
		var servers []struct {
			DSN string `json:"dsn"` // Corrected json tag
		}
		err = json.Unmarshal(content, &servers)
		if err != nil {
			fmt.Printf("Failed to parse JSON: %v\n", err)
			os.Exit(1)
		}
		for _, s := range servers {
			dsnToUse := s.DSN
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
	if *sqlFile != "" {
		content, err := os.ReadFile(*sqlFile)
		if err != nil {
			fmt.Printf("Failed to read SQL file: %v\n", err)
			os.Exit(1)
		}
		sqls = string(content)
	} else if *file != "" {
		content, err := os.ReadFile(*file)
		if err != nil {
			fmt.Printf("Failed to read file: %v\n", err)
			os.Exit(1)
		}
		sqls = string(content)
	} else if *statements != "" {
		sqls = *statements
	} else {
		fmt.Println("Error: must provide --sqlfile, --file, or --statements")
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
