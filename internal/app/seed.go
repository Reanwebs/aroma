package app

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

// SeedDevelopment creates only the local development login. It never replaces
// an existing account or runs outside APP_ENV=development.
func SeedDevelopment(db *gorm.DB) error {
	var count int64
	if err := db.Model(&User{}).Where("email = ?", "admin@example.com").Count(&count).Error; err != nil || count > 0 {
		return err
	}
	hash, err := hash("password")
	if err != nil {
		return err
	}
	return db.Create(&User{Name: "Development Admin", Email: "admin@example.com", PasswordHash: hash, Role: "SUPER_ADMIN", IsActive: true}).Error
}

// SeedInitialAdmin creates the first production administrator only when the
// user table is empty. Remove the bootstrap password from the deployment
// environment after the first successful startup.
func SeedInitialAdmin(db *gorm.DB, name, email, password string) error {
	if strings.TrimSpace(email) == "" || len(password) < 12 {
		return errors.New("initial admin email and a password of at least 12 characters are required")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&User{}).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		hashedPassword, err := hash(password)
		if err != nil {
			return err
		}
		return tx.Create(&User{
			Name:         strings.TrimSpace(name),
			Email:        strings.TrimSpace(email),
			PasswordHash: hashedPassword,
			Role:         "SUPER_ADMIN",
			IsActive:     true,
		}).Error
	})
}
