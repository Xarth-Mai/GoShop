package core

import (
	"time"

	"GoShop/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	DB        *gorm.DB // 主库（写与读）
	ReplicaDB *gorm.DB // 从库（仅读）
)

// InitDB 初始化数据库连接（主从读写分离）
func InitDB() error {
	var err error
	cfg := config.GlobalConfig.Database

	// 初始化主库
	DB, err = gorm.Open(postgres.Open(cfg.Master), &gorm.Config{})
	if err != nil {
		return err
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)

	// 如果配置了从库，则初始化从库连接
	if cfg.Replica != "" && cfg.Replica != cfg.Master {
		ReplicaDB, err = gorm.Open(postgres.Open(cfg.Replica), &gorm.Config{})
		if err != nil {
			return err
		}
		sqlReplicaDB, err := ReplicaDB.DB()
		if err != nil {
			return err
		}
		sqlReplicaDB.SetMaxIdleConns(cfg.MaxIdleConns)
		sqlReplicaDB.SetMaxOpenConns(cfg.MaxOpenConns)
		sqlReplicaDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)
	} else {
		// 没有独立的从库，则直接复用主库
		ReplicaDB = DB
	}

	return nil
}
