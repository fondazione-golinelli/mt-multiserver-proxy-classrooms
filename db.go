package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// openDB opens a connection pool to the configured MySQL database.
func openDB(cfg classroomsConfig) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=true&loc=UTC",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	log.Printf("[%s] connected to MySQL %s/%s", pluginName, cfg.DBHost, cfg.DBName)
	return db, nil
}

// migrateDB creates the schema tables if they don't already exist.
func migrateDB(db *sql.DB) error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS teachers (
			username   VARCHAR(50) PRIMARY KEY,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE IF NOT EXISTS classes (
			id         INT AUTO_INCREMENT PRIMARY KEY,
			name       VARCHAR(100) NOT NULL UNIQUE,
			created_by VARCHAR(50) NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (created_by) REFERENCES teachers(username)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE IF NOT EXISTS class_students (
			class_id INT         NOT NULL,
			username VARCHAR(50) NOT NULL,
			PRIMARY KEY (class_id, username),
			FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE IF NOT EXISTS instances (
			id            VARCHAR(100) PRIMARY KEY,
			class_id      INT,
			created_by    VARCHAR(50)  NOT NULL,
			template_name VARCHAR(100) NOT NULL,
			created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
			server_id     INT          NOT NULL,
			uuid          VARCHAR(36)  NOT NULL,
			node_id       INT          NOT NULL,
			proxy_name    VARCHAR(200) NOT NULL,
			backend_addr  VARCHAR(200) NOT NULL,
			status        VARCHAR(20)  NOT NULL DEFAULT 'provisioning',
			FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE SET NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE IF NOT EXISTS instance_invites (
			instance_id VARCHAR(100) NOT NULL,
			username    VARCHAR(50)  NOT NULL,
			PRIMARY KEY (instance_id, username),
			FOREIGN KEY (instance_id) REFERENCES instances(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}

	for _, stmt := range migrations {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w\nstatement: %s", err, stmt)
		}
	}

	log.Printf("[%s] database migration complete", pluginName)
	return nil
}
