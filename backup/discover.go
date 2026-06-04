package backup

import (
	"database/sql"
	"fmt"

	"godump/config"

	_ "github.com/go-sql-driver/mysql"
)

func discoverDatabases(cfg config.InstanceConfig) ([]string, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/?parseTime=true", cfg.User, cfg.Password, cfg.Host, cfg.Port)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	// Ensure the connection is actually valid
	if err := db.Ping(); err != nil {
		return nil, err
	}

	query := `
		SELECT SCHEMA_NAME 
		FROM INFORMATION_SCHEMA.SCHEMATA 
		WHERE SCHEMA_NAME NOT IN ('information_schema', 'performance_schema', 'mysql', 'sys')
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var databases []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		databases = append(databases, name)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return databases, nil
}
