package storage

import (
	"fmt"
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"go-serverhttp-template/server/config"
)

var DB *gorm.DB

func InitDB() error {
	conf := config.LoadConfig()
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		conf.MysqlConfig.Host, conf.MysqlConfig.Port, conf.MysqlConfig, conf.MysqlConfig.Password, conf.MysqlConfig.DBName)
	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
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
