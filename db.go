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
			institute  VARCHAR(100) NOT NULL DEFAULT '',
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
			UNIQUE KEY uniq_class_students_username (username),
			FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE IF NOT EXISTS instances (
			id            VARCHAR(100) PRIMARY KEY,
			class_id      INT,
			created_by    VARCHAR(50)  NOT NULL,
			institute     VARCHAR(100) NOT NULL DEFAULT '',
			display_name  VARCHAR(100) NOT NULL DEFAULT '',
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

	if err := addColumnIfMissing(db, "teachers", "institute",
		"ALTER TABLE teachers ADD COLUMN institute VARCHAR(100) NOT NULL DEFAULT '' AFTER username"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "instances", "institute",
		"ALTER TABLE instances ADD COLUMN institute VARCHAR(100) NOT NULL DEFAULT '' AFTER created_by"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "instances", "display_name",
		"ALTER TABLE instances ADD COLUMN display_name VARCHAR(100) NOT NULL DEFAULT '' AFTER institute"); err != nil {
		return err
	}
	if err := addUniqueIndexIfMissing(db, "class_students", "uniq_class_students_username",
		"ALTER TABLE class_students ADD UNIQUE KEY uniq_class_students_username (username)"); err != nil {
		return err
	}

	log.Printf("[%s] database migration complete", pluginName)
	return nil
}

func addColumnIfMissing(db *sql.DB, tableName, columnName, stmt string) error {
	var exists int
	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
			AND TABLE_NAME = ?
			AND COLUMN_NAME = ?`, tableName, columnName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check column %s.%s: %w", tableName, columnName, err)
	}
	if exists > 0 {
		return nil
	}
	if _, err := db.Exec(stmt); err != nil {
		return fmt.Errorf("migrate column %s.%s: %w", tableName, columnName, err)
	}
	return nil
}

func addUniqueIndexIfMissing(db *sql.DB, tableName, indexName, stmt string) error {
	var exists int
	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
			AND TABLE_NAME = ?
			AND INDEX_NAME = ?`, tableName, indexName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check index %s.%s: %w", tableName, indexName, err)
	}
	if exists > 0 {
		return nil
	}
	if _, err := db.Exec(stmt); err != nil {
		return fmt.Errorf("migrate index %s.%s: %w", tableName, indexName, err)
	}
	return nil
}
