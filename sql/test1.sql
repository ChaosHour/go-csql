-- Test file for go-csql verbosity and functionality testing

-- Simple select query
SELECT 1 as test_column;

-- Show current database information
SELECT DATABASE() as current_database, CURRENT_USER() as `current_user`, VERSION() as mysql_version;

-- List all databases
SHOW DATABASES;

-- Get current connection information
SELECT CONNECTION_ID() as connection_id, @@hostname as server_hostname;

-- Show table count in current database
SELECT COUNT(*) as table_count FROM information_schema.tables WHERE table_schema = DATABASE();

-- Show some system variables
SHOW VARIABLES LIKE 'max_connections';

-- Simple math operations
SELECT 
    1 + 1 as addition,
    10 * 5 as multiplication,
    NOW() as `current_timestamp`;

-- Check user privileges
SELECT COUNT(*) as privilege_count FROM information_schema.user_privileges WHERE GRANTEE LIKE CONCAT("'", CURRENT_USER(), "'");
