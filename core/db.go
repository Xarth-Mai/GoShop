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

// InitDB 初始化数据库连接（支持主从读写分离与微服务多库隔离）
func InitDB(serviceName string) error {
	var err error
	cfg := config.GlobalConfig.Database

	dsn := cfg.Master
	// 容错自愈：如果配置了微服务专有连接，先尝试使用专有连接
	if serviceName != "" && cfg.Services != nil && cfg.Services[serviceName] != "" {
		dsn = cfg.Services[serviceName]
	}

	// 初始化主库
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		// 降级策略：如果微服务专属库未建立（如本地单库调试），自动降级回退到 master 共享大库
		DB, err = gorm.Open(postgres.Open(cfg.Master), &gorm.Config{})
		if err != nil {
			return err
		}
	}

	// 根据当前服务进行局部的 Model 自动迁移，防止各服务交叉建表
	var migrateModels []interface{}
	switch serviceName {
	case "goshop-user-service":
		migrateModels = []interface{}{&models.User{}, &models.Address{}}
	case "goshop-product-service":
		migrateModels = []interface{}{&models.Category{}, &models.Spu{}, &models.Sku{}}
	case "goshop-inventory-service":
		migrateModels = []interface{}{&models.SkuInventory{}, &models.InventoryReservation{}, &models.InventoryJournal{}}
	case "goshop-promotion-service":
		migrateModels = []interface{}{&models.Coupon{}, &models.UserCoupon{}}
	case "goshop-order-service":
		migrateModels = []interface{}{
			&models.Order{}, &models.OrderItem{}, &models.OrderPromotionAllocation{},
			&models.OrderStateLog{}, &models.DeadLetterOrder{}, &models.InboxEvent{}, &models.OutboxEvent{},
		}
	case "goshop-payment-service":
		migrateModels = []interface{}{&models.PaymentOrder{}, &models.PaymentTransaction{}, &models.InboxEvent{}, &models.OutboxEvent{}}
	case "goshop-aftersale-service":
		migrateModels = []interface{}{
			&models.AfterSaleOrder{}, &models.AfterSaleItem{}, &models.RefundOrder{},
			&models.AccountingEntry{}, &models.InboxEvent{}, &models.OutboxEvent{},
		}
	case "goshop-cart-service":
		migrateModels = []interface{}{&models.CartItem{}}
	case "goshop-scheduler-service":
		migrateModels = []interface{}{&models.OutboxEvent{}}
	default:
		// 兜底（空服务名，如单体运行/测试），进行全量 Model 自动迁移，保持向下兼容
		migrateModels = []interface{}{
			&models.User{}, &models.Category{}, &models.Spu{}, &models.Sku{},
			&models.Order{}, &models.OrderItem{}, &models.OrderPromotionAllocation{},
			&models.OrderStateLog{}, &models.PaymentOrder{}, &models.PaymentTransaction{},
			&models.RefundOrder{}, &models.AccountingEntry{}, &models.AfterSaleOrder{},
			&models.AfterSaleItem{}, &models.SkuInventory{}, &models.InventoryReservation{},
			&models.InventoryJournal{}, &models.OutboxEvent{}, &models.InboxEvent{},
			&models.Address{}, &models.Coupon{}, &models.UserCoupon{}, &models.CartItem{},
			&models.DeadLetterOrder{},
		}
	}

	if len(migrateModels) > 0 {
		if err = DB.AutoMigrate(migrateModels...); err != nil {
			return err
		}
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
