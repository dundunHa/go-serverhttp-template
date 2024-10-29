package storage

import (
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"go-serverhttp-template/server/config"
)

var DB *gorm.DB

func InitDB() error {
	conf := config.LoadConfig()
	var err error
	DB, err = gorm.Open(postgres.Open(conf.DB.Mysql.Addr), &gorm.Config{})
	if err != nil {
		return err
	}

	// 自动迁移数据库表
	//if err := DB.AutoMigrate(&model.example{}); err != nil {
	//	return err
	//}

	log.Println("PostgreSQL Database initialized and migrated successfully.")
	return nil
}
