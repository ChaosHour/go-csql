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

```

**2. Instances via Flags, SQL from File (`--file`)**

```bash
# statements.sql contains:
# SELECT * FROM users;
# SELECT COUNT(*) FROM orders;

./bin/go-csql --instances="user:pass@tcp(host1:3306)/db1" --file=statements.sql
```

**3. Instances via Flags, SQL from Text File (`--sqlfile`)**

This is similar to `--file` but might be preferred for clarity.

```bash
# queries.txt contains:
# SELECT * FROM products;
# DELETE FROM logs WHERE timestamp < NOW() - INTERVAL 1 DAY;

./bin/go-csql --instances="user:pass@tcp(host1:3306)/db1" --sqlfile=queries.txt
```

**4. Instances from JSON File (`--json`), Statements via Flags**

```bash
# servers.json contains:
# [
#   {"dsn": "user:pass@tcp(host1:3306)/db1"},
#   {"dsn": "user:pass@tcp(host2:3306)/db2"}
# ]

./bin/go-csql --json=servers.json --statements="SELECT @@hostname;"
```

**5. Instances from JSON, SQL from Text File

```bash
./bin/go-csql --json=servers.json --sqlfile=queries.txt
```

**6. Using `~/.my.cnf` Credentials**

If you have a `~/.my.cnf` file with `[client]` credentials (user, password, host, port, database), the CLI will automatically use them to fill in *missing* parts of the DSN provided via `--instances` or `--json`. Host/port from `.my.cnf` are only used if not specified in the DSN.

```bash
# ~/.my.cnf might contain:
# [client]
# user=myuser
# password=mypass
# host=db.example.com

# You can then omit credentials/host if they match .my.cnf:
./bin/go-csql --instances="@tcp(:3306)/db1" --statements="SELECT 1"
# This would connect using myuser:mypass@tcp(db.example.com:3306)/db1

./bin/go-csql --instances="@tcp(:3307)/mysql" --statements="show slave status\G" | awk -v RS='\n ' '
{
    if ($1 ~ /Master_Host|Slave_IO_Running|Slave_SQL_Running|Seconds_Behind_Master|Retrieved_Gtid_Set|Executed_Gtid_Set/) {
        split($0, a, ": ");
        print a[1] ": " substr($0, index($0, a[2]));
    }
}'
                 Master_Host: 172.20.0.3
            Slave_IO_Running: Yes
           Slave_SQL_Running: Yes
       Seconds_Behind_Master: 0
Master_SSL_Verify_Server_Cert: No
     Slave_SQL_Running_State: Replica has read all relay log; waiting for more updates
          Retrieved_Gtid_Set: 6ef3e484-1e68-11f0-8a88-d66d6a16a219:1-196
           Executed_Gtid_Set: 6ef3e484-1e68-11f0-8a88-d66d6a16a219:1-196,
6ef56be4-1e68-11f0-97f4-7a1c5884f911:1-11
```

**7. Disabling Concurrency

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
