package backup

import (
	"database/sql"
	"fmt"

	"godump/config"

	_ "github.com/go-sql-driver/mysql"
)

func discoverDatabases(db *sql.DB, cfg config.InstanceConfig) ([]string, error) {
	if db == nil {
		return nil, fmt.Errorf("database pool is not initialized")
	}

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

		if len(cfg.Include) > 0 {
			included := false
			for _, inc := range cfg.Include {
				if name == inc {
					included = true
					break
				}
			}
			if !included {
				continue
			}
		}

		if len(cfg.Exclude) > 0 {
			excluded := false
			for _, exc := range cfg.Exclude {
				if name == exc {
					excluded = true
					break
				}
			}
			if excluded {
				continue
			}
		}

		databases = append(databases, name)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return databases, nil
}
