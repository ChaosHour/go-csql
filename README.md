# go-csql

A simple Go CLI for connecting to one or more MySQL CloudSQL instances and running one or more SQL statements.

## Features

- Connect to multiple MySQL instances (CloudSQL or standard MySQL)
- Run one or more SQL statements (from CLI or file)
- Print results to the console

## Usage

### Build

Build the binary into the `./bin` directory:

```bash
make build
```

### Examples

**1. Basic: Instances and Statements via Flags

Run specific statements against comma-separated instances:

```bash
./bin/go-csql --instances="user:pass@tcp(host1:3306)/db1,user:pass@tcp(host2:3306)/db2" \
           --statements="SELECT version();SHOW TABLES"
```

**2. Reading SQL from stdin (pipe support)

Pipe SQL statements directly into go-csql:

```bash
cat test.sql | ./bin/go-csql --instances="root:s3cr3t@tcp(192.168.50.50:3306)/mysql" --stdin

# With verbosity (planned features similar to MySQL client):
cat test.sql | ./bin/go-csql --instances="root:s3cr3t@tcp(192.168.50.50:3306)/mysql" --stdin -v
# -v: Shows executed statements with -------------- separators

cat test.sql | ./bin/go-csql --instances="root:s3cr3t@tcp(192.168.50.50:3306)/mysql" --stdin -vv  
# -vv: Shows statements, separators, and row count information

cat test.sql | ./bin/go-csql --instances="root:s3cr3t@tcp(192.168.50.50:3306)/mysql" --stdin -vvv
# -vvv: Shows statements, separators, table format, and timing information
```

**Note:** Verbosity flags are currently parsed but not fully implemented. The MySQL-style verbose output with statement separators and timing information requires updates to the database package.

**3. Example output with verbosity:**

```bash
# Without verbosity:
./bin/go-csql --instances="root:s3cr3t@tcp(192.168.50.50:3306)/mysql,root:s3cr3t@tcp(192.168.50.50:3307)/mysql" \
           --table --statements="show databases"
Executing statements on 2 instance(s) (concurrent: true)...
[root:s3cr3t@tcp(192.168.50.50:3307)/mysql] show databases
+--------------------+
| DATABASE           |
+--------------------+
| chaos              |
| information_schema |
| mysql              |
| performance_schema |
| sys                |
+--------------------+
---
[root:s3cr3t@tcp(192.168.50.50:3306)/mysql] show databases
+--------------------+
| DATABASE           |
+--------------------+
| chaos              |
| information_schema |
| mysql              |
| performance_schema |
| sys                |
+--------------------+
---
All executions complete.

# With verbosity (-v):
# Shows "-------------" separators and executed statements before results
```

**4. Instances via Flags, SQL from File (`--file`)**

```bash
# statements.sql contains:
# SELECT * FROM users;
# SELECT COUNT(*) FROM orders;

./bin/go-csql --instances="user:pass@tcp(host1:3306)/db1" --file=statements.sql
```

**5. Instances via Flags, SQL from Text File (`--sqlfile`)**

This is similar to `--file` but might be preferred for clarity.

```bash
# queries.txt contains:
# SELECT * FROM products;
# DELETE FROM logs WHERE timestamp < NOW() - INTERVAL 1 DAY;

./bin/go-csql --instances="user:pass@tcp(host1:3306)/db1" --sqlfile=queries.txt
```

**6. Instances from JSON File (`--json`), Statements via Flags**

```bash
# servers.json supports two formats:

# Format 1: Individual components (recommended for complex passwords)
# [
#   {
#     "user": "myuser",
#     "password": "my@complex#password!$with&symbols",
#     "host": "host1",
#     "port": "3306",
#     "database": "db1"
#   },
#   {
#     "user": "user2", 
#     "password": "another!complex@password#123",
#     "host": "host2",
#     "port": "3306", 
#     "database": "db2"
#   }
# ]

# Format 2: Traditional DSN format (for simple passwords)
# [
#   {"dsn": "user:simplepass@tcp(host1:3306)/db1"},
#   {"dsn": "user2:pass123@tcp(host2:3306)/db2"}
# ]

# Using tilde (~) for home directory paths:
./bin/go-csql --json=~/.servers.json --statements="SELECT @@hostname;"
./bin/go-csql --json="~/.servers.json" --statements="SELECT @@hostname;"

./bin/go-csql --json=servers.json --statements="SELECT @@hostname;"
```

**Note:** For complex passwords with special characters (@, #, !, $, &, etc.), use the individual component format in your JSON file. Passwords are automatically URL-encoded internally. JSON files support comments (lines starting with # that don't contain JSON syntax) for documentation purposes. Passwords that start with # are fully supported.

**Example with password starting with #:**

```json
[
  {
    "user": "testuser",
    "password": "#complex!password@123",
    "host": "localhost",
    "port": "3306",
    "database": "testdb"
  }
]
```

**7. Instances from JSON, SQL from Text File

```bash
./bin/go-csql --json=servers.json --sqlfile=queries.txt
```

**8. Using `~/.my.cnf` Credentials**

If you have a `~/.my.cnf` file with `[client]` credentials (user, password, host, port, database), the CLI will automatically use them to fill in *missing* parts of the DSN provided via `--instances` or `--json`. Host/port from `.my.cnf` are only used if not specified in the DSN.

```bash
# ~/.my.cnf might contain (complex passwords supported):
# [client]
# user=myuser
# password=my@complex#password!
# host=db.example.com

# You can then omit credentials/host if they match .my.cnf:
./bin/go-csql --instances="@tcp(:3306)/db1" --statements="SELECT 1"
# This would connect using myuser with the complex password from .my.cnf
```

**9. Disabling Concurrency

Run queries sequentially against each instance instead of concurrently:

```bash
./bin/go-csql --instances="inst1,inst2" --statements="SELECT 1" --concurrent=false
```

### Docker

Build the Docker image:

```bash
make docker-build
```

Run the CLI using the Docker image (mount current directory for file access):

```bash
docker run --rm -v "$(pwd):/app" -w /app ghcr.io/chaoshour/go-csql:latest \
       --json=servers.json \
       --sqlfile=queries.txt
```

## Development

- CLI entry: `cmd/csql/main.go`
- Database logic: `pkg/db/db.go`

## Requirements

- Go 1.18+
- MySQL driver (installed automatically via Go modules)

## Inspired by

- [ccql](https://github.com/github/ccql)

Thank you! Shlomi Noach
