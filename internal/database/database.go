package database

import (
	"embed"
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

//go:embed migrations/*.sql
var migrations embed.FS

func Open(dsn string) (*gorm.DB, error) { return gorm.Open(postgres.Open(dsn), &gorm.Config{}) }
func Migrate(db *gorm.DB) error {
	sql, e := migrations.ReadFile("migrations/000001_initial.up.sql")
	if e != nil {
		return e
	}
	if e = db.Exec("CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())").Error; e != nil {
		return e
	}
	var n int64
	db.Raw("SELECT count(*) FROM schema_migrations WHERE version='000001'").Scan(&n)
	if n == 0 {
		if db.Raw("SELECT count(*) FROM information_schema.tables WHERE table_name='users'").Scan(&n); n > 0 {
			if e = db.Exec("INSERT INTO schema_migrations(version) VALUES ('000001')").Error; e != nil {
				return e
			}
		} else {
			if e = db.Exec(string(sql)).Error; e != nil {
				return fmt.Errorf("apply migration 000001: %w", e)
			}
			if e = db.Exec("INSERT INTO schema_migrations(version) VALUES ('000001')").Error; e != nil {
				return e
			}
		}
	}
	if e = db.Exec("ALTER TABLE customers ADD COLUMN IF NOT EXISTS whatsapp_number text").Error; e != nil {
		return e
	}
	var applied int64
	if err := db.Raw("SELECT count(*) FROM schema_migrations WHERE version='000002'").Scan(&applied).Error; err != nil {
		return err
	}
	if applied == 0 {
		sql, err := migrations.ReadFile("migrations/000002_market_price_snapshots.up.sql")
		if err != nil {
			return err
		}
		if err := db.Exec(string(sql)).Error; err != nil {
			return fmt.Errorf("apply migration 000002: %w", err)
		}
		if err := db.Exec("INSERT INTO schema_migrations(version) VALUES ('000002')").Error; err != nil {
			return err
		}
	}
	if err := db.Raw("SELECT count(*) FROM schema_migrations WHERE version='000003'").Scan(&applied).Error; err != nil {
		return err
	}
	if applied == 0 {
		sql, err := migrations.ReadFile("migrations/000003_customer_profile_image.up.sql")
		if err != nil {
			return err
		}
		if err := db.Exec(string(sql)).Error; err != nil {
			return fmt.Errorf("apply migration 000003: %w", err)
		}
		if err := db.Exec("INSERT INTO schema_migrations(version) VALUES ('000003')").Error; err != nil {
			return err
		}
	}
	if err := db.Raw("SELECT count(*) FROM schema_migrations WHERE version='000004'").Scan(&applied).Error; err != nil {
		return err
	}
	if applied == 0 {
		sql, err := migrations.ReadFile("migrations/000004_job_bag_count.up.sql")
		if err != nil {
			return err
		}
		if err := db.Exec(string(sql)).Error; err != nil {
			return fmt.Errorf("apply migration 000004: %w", err)
		}
		if err := db.Exec("INSERT INTO schema_migrations(version) VALUES ('000004')").Error; err != nil {
			return err
		}
	}
	return nil
}
