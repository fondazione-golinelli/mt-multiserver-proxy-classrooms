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

		`CREATE TABLE IF NOT EXISTS class_teachers (
			class_id INT         NOT NULL,
			username VARCHAR(50) NOT NULL,
			PRIMARY KEY (class_id, username),
			FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE CASCADE,
			FOREIGN KEY (username) REFERENCES teachers(username) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE IF NOT EXISTS class_assistants (
			class_id INT         NOT NULL,
			username VARCHAR(50) NOT NULL,
			PRIMARY KEY (class_id, username),
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

		`CREATE TABLE IF NOT EXISTS instance_settings (
			instance_id              VARCHAR(100) PRIMARY KEY,
			enable_damage            TINYINT(1) NOT NULL DEFAULT 0,
			enable_pvp               TINYINT(1) NOT NULL DEFAULT 0,
			mcl_enable_hunger        TINYINT(1) NOT NULL DEFAULT 0,
			mobs_spawn               TINYINT(1) NOT NULL DEFAULT 0,
			only_peaceful_mobs       TINYINT(1) NOT NULL DEFAULT 0,
			mcl_explosions_griefing  TINYINT(1) NOT NULL DEFAULT 0,
			static_spawnpoint        VARCHAR(100) DEFAULT NULL,
			spawn_yaw                DOUBLE DEFAULT NULL,
			spawn_pitch              DOUBLE DEFAULT NULL,
			FOREIGN KEY (instance_id) REFERENCES instances(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE IF NOT EXISTS class_groups (
			id       INT AUTO_INCREMENT PRIMARY KEY,
			class_id INT         NOT NULL,
			name     VARCHAR(50) NOT NULL,
			color    VARCHAR(9)  NOT NULL,
			UNIQUE KEY uniq_class_group_name (class_id, name),
			FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE IF NOT EXISTS class_group_members (
			class_id INT         NOT NULL,
			username VARCHAR(50) NOT NULL,
			group_id INT         NOT NULL,
			PRIMARY KEY (class_id, username),
			FOREIGN KEY (group_id) REFERENCES class_groups(id) ON DELETE CASCADE,
			FOREIGN KEY (class_id, username) REFERENCES class_students(class_id, username) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE IF NOT EXISTS instance_zones (
			id          INT AUTO_INCREMENT PRIMARY KEY,
			instance_id VARCHAR(100) NOT NULL,
			name        VARCHAR(50)  NOT NULL,
			group_id    INT          DEFAULT NULL,
			min_x       INT          NOT NULL,
			min_z       INT          NOT NULL,
			max_x       INT          NOT NULL,
			max_z       INT          NOT NULL,
			FOREIGN KEY (instance_id) REFERENCES instances(id) ON DELETE CASCADE,
			FOREIGN KEY (group_id) REFERENCES class_groups(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE IF NOT EXISTS zone_missions (
			id           INT AUTO_INCREMENT PRIMARY KEY,
			zone_id      INT          NOT NULL UNIQUE,
			title        VARCHAR(80)  NOT NULL,
			description  VARCHAR(255) NOT NULL DEFAULT '',
			objectives   TEXT         NOT NULL,
			tools        TEXT         NOT NULL,
			created_at   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
			completed_at TIMESTAMP    NULL DEFAULT NULL,
			FOREIGN KEY (zone_id) REFERENCES instance_zones(id) ON DELETE CASCADE
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
	if err := addColumnIfMissing(db, "instance_zones", "open_access",
		"ALTER TABLE instance_zones ADD COLUMN open_access TINYINT(1) NOT NULL DEFAULT 0 AFTER group_id"); err != nil {
		return err
	}
	for _, col := range []string{"tp_x", "tp_y", "tp_z", "tp_yaw"} {
		if err := addColumnIfMissing(db, "instance_zones", col,
			"ALTER TABLE instance_zones ADD COLUMN "+col+" DOUBLE DEFAULT NULL"); err != nil {
			return err
		}
	}
	if err := addColumnIfMissing(db, "instance_settings", "enable_pvp",
		"ALTER TABLE instance_settings ADD COLUMN enable_pvp TINYINT(1) NOT NULL DEFAULT 0 AFTER enable_damage"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "instance_settings", "mobs_spawn",
		"ALTER TABLE instance_settings ADD COLUMN mobs_spawn TINYINT(1) NOT NULL DEFAULT 0 AFTER mcl_enable_hunger"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "instance_settings", "only_peaceful_mobs",
		"ALTER TABLE instance_settings ADD COLUMN only_peaceful_mobs TINYINT(1) NOT NULL DEFAULT 0 AFTER mobs_spawn"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "instance_settings", "static_spawnpoint",
		"ALTER TABLE instance_settings ADD COLUMN static_spawnpoint VARCHAR(100) DEFAULT NULL AFTER mcl_explosions_griefing"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "instance_settings", "spawn_yaw",
		"ALTER TABLE instance_settings ADD COLUMN spawn_yaw DOUBLE DEFAULT NULL AFTER static_spawnpoint"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "instance_settings", "spawn_pitch",
		"ALTER TABLE instance_settings ADD COLUMN spawn_pitch DOUBLE DEFAULT NULL AFTER spawn_yaw"); err != nil {
		return err
	}
	for _, col := range []string{"student_fly", "student_creative"} {
		if err := addColumnIfMissing(db, "instance_settings", col,
			"ALTER TABLE instance_settings ADD COLUMN "+col+" TINYINT(1) NOT NULL DEFAULT 0"); err != nil {
			return err
		}
	}
	// World missions: zone_id NULL, bound to the world (and optionally a group).
	if err := addColumnIfMissing(db, "zone_missions", "instance_id",
		`ALTER TABLE zone_missions ADD COLUMN instance_id VARCHAR(100) DEFAULT NULL AFTER zone_id,
			ADD CONSTRAINT fk_zone_missions_instance FOREIGN KEY (instance_id) REFERENCES instances(id) ON DELETE CASCADE`); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "zone_missions", "group_id",
		`ALTER TABLE zone_missions ADD COLUMN group_id INT DEFAULT NULL AFTER instance_id,
			ADD CONSTRAINT fk_zone_missions_group FOREIGN KEY (group_id) REFERENCES class_groups(id) ON DELETE SET NULL`); err != nil {
		return err
	}
	var zoneNullable string
	if err := db.QueryRow(`SELECT IS_NULLABLE FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'zone_missions' AND COLUMN_NAME = 'zone_id'`).Scan(&zoneNullable); err != nil {
		return fmt.Errorf("check column zone_missions.zone_id: %w", err)
	}
	if zoneNullable != "YES" {
		if _, err := db.Exec("ALTER TABLE zone_missions MODIFY zone_id INT NULL DEFAULT NULL"); err != nil {
			return fmt.Errorf("migrate column zone_missions.zone_id: %w", err)
		}
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
