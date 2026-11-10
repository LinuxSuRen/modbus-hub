package store

import (
	"path/filepath"

	"github.com/glebarez/sqlite"
	"github.com/linuxsuren/modbus-hub/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open 打开（必要时创建）SQLite 数据库并完成建表与单例配置初始化。
func Open(dataDir string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(dataDir, "modbus-hub.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(
		&model.Connection{}, &model.Point{},
		&model.SinkMQTT{}, &model.SinkTCP{},
	); err != nil {
		return nil, err
	}
	// 单例转发配置：固定 ID=1，缺省即建
	var cnt int64
	if err := db.Model(&model.SinkMQTT{}).Count(&cnt).Error; err != nil {
		return nil, err
	}
	if cnt == 0 {
		if err := db.Create(&model.SinkMQTT{
			ID: 1, Broker: "tcp://127.0.0.1:1883",
			ClientID: "modbus-hub", TopicPrefix: "modbus",
		}).Error; err != nil {
			return nil, err
		}
	}
	if err := db.Model(&model.SinkTCP{}).Count(&cnt).Error; err != nil {
		return nil, err
	}
	if cnt == 0 {
		if err := db.Create(&model.SinkTCP{ID: 1, Listen: ":9000"}).Error; err != nil {
			return nil, err
		}
	}
	return db, nil
}
