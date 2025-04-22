package storage

import (
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"go-serverhttp-template/internal/config"
)

var DB *gorm.DB

func InitDB() error {
	conf, err := config.LoadConfig()
	if err != nil {
		return err
	}

	DB, err = gorm.Open(postgres.Open(conf.DB.Mysql.Addr), &gorm.Config{})
	if err != nil {
		return err
	}

	//if err := DB.AutoMigrate(&model.example{}); err != nil {
	//	return err
	//}

	log.Println("PostgreSQL Database initialized and migrated successfully.")
	return nil
}
