package core

import (
	"time"

	"GoShop/config"
	"GoShop/models"

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

	// 数据库自动迁移
	err = DB.AutoMigrate(
		&models.User{},
		&models.Category{},
		&models.Spu{},
		&models.Sku{},
		&models.Order{},
		&models.OrderItem{},
		&models.OrderPromotionAllocation{},
		&models.OrderStateLog{},
		&models.PaymentOrder{},
		&models.PaymentTransaction{},
		&models.RefundOrder{},
		&models.AccountingEntry{},
		&models.AfterSaleOrder{},
		&models.AfterSaleItem{},
		&models.SkuInventory{},
		&models.InventoryReservation{},
		&models.InventoryJournal{},
		&models.Address{},
		&models.Coupon{},
		&models.UserCoupon{},
		&models.CartItem{},
		&models.DeadLetterOrder{},
	)
	if err != nil {
		return err
	}
	if err := migrateLegacyOrderStatuses(DB); err != nil {
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

func migrateLegacyOrderStatuses(db *gorm.DB) error {
	statements := []string{
		"UPDATE orders SET status = 10, pay_status = 0, after_sale_status = 0 WHERE status = 1",
		"UPDATE orders SET status = 20, pay_status = 20, after_sale_status = 0 WHERE status = 2",
		"UPDATE orders SET status = 60 WHERE status = 3",
		"UPDATE orders SET status = 110, pay_status = 20, after_sale_status = 10 WHERE status = 4",
		"UPDATE orders SET status = 120, pay_status = 40, after_sale_status = 70 WHERE status = 5",
		"UPDATE orders SET status = 130, pay_status = 20, after_sale_status = 30 WHERE status = 6",
	}
	for _, stmt := range statements {
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}
