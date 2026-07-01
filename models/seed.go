package models

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const SeedTestUserPasswordHash = "$2a$10$jsyjKfwWcrG/7lnKRYA75.sgLsRQPMc.qtLE0NW.VgLuoYNGTOA7u" // 123456

func SeedServiceData(db *gorm.DB, serviceName string) error {
	switch serviceName {
	case "goshop-user-service":
		userID, err := SeedUserData(db)
		if err != nil {
			return err
		}
		return SeedAddressData(db, userID)
	case "goshop-product-service":
		return SeedProductCatalog(db)
	case "goshop-inventory-service":
		return SeedInventoryData(db)
	case "goshop-promotion-service":
		return SeedPromotionData(db, 1)
	case "":
		userID, err := SeedUserData(db)
		if err != nil {
			return err
		}
		if err := SeedProductCatalog(db); err != nil {
			return err
		}
		if err := SeedPromotionData(db, userID); err != nil {
			return err
		}
		if err := SeedAddressData(db, userID); err != nil {
			return err
		}
		return SeedInventoryData(db)
	default:
		return nil
	}
}

func SeedUserData(db *gorm.DB) (uint, error) {
	var testUser User
	err := db.Where("username = ?", "test_user").First(&testUser).Error
	if err == nil {
		if testUser.PasswordHash != SeedTestUserPasswordHash {
			testUser.PasswordHash = SeedTestUserPasswordHash
			if err := db.Save(&testUser).Error; err != nil {
				return 0, err
			}
		}
		return testUser.ID, nil
	}
	if err != gorm.ErrRecordNotFound {
		return 0, err
	}

	testUser = User{
		Username:     "test_user",
		PasswordHash: SeedTestUserPasswordHash,
		Email:        "test@example.com",
		Role:         UserRoleUser,
	}
	if err := db.Create(&testUser).Error; err != nil {
		return 0, err
	}
	return testUser.ID, nil
}

func SeedProductCatalog(db *gorm.DB) error {
	categories := []Category{
		{ID: 1, Name: "智能手机", SortOrder: 1},
		{ID: 2, Name: "笔记本", SortOrder: 2},
		{ID: 3, Name: "穿戴数码", SortOrder: 3},
		{ID: 4, Name: "智能平板", SortOrder: 4},
	}
	for _, category := range categories {
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&category).Error; err != nil {
			return err
		}
	}

	spus := []Spu{
		{
			ID:          1,
			CategoryID:  1,
			Name:        "Claude Phone 1",
			Subtitle:    "懂你的思考伙伴，掌上轻量体验",
			Description: "Claude Phone 1 采用极简设计与温暖配色。搭载端侧小模型，无论是日常事务处理还是深度人机对话，都是您最得力的助手。",
			MainImage:   "https://images.unsplash.com/photo-1511707171634-5f897ff02aa9?auto=format&fit=crop&w=800&q=80",
			Images:      `["https://images.unsplash.com/photo-1511707171634-5f897ff02aa9"]`,
			DetailHTML:  "<p>Claude Phone 1 详细介绍内容...</p>",
			Status:      1,
		},
		{
			ID:          2,
			CategoryID:  2,
			Name:        "Anthropic Book Pro",
			Subtitle:    "极致性能，为灵感创作而生",
			Description: "搭载专为大模型应用优化的新一代芯片，Anthropic Book Pro 拥有超凡的运算能力与极长续航。",
			MainImage:   "https://images.unsplash.com/photo-1496181130204-755241544e3f?auto=format&fit=crop&w=800&q=80",
			Images:      `["https://images.unsplash.com/photo-1496181130204-755241544e3f"]`,
			DetailHTML:  "<p>Anthropic Book Pro 详细介绍内容...</p>",
			Status:      1,
		},
		{
			ID:          3,
			CategoryID:  3,
			Name:        "Artifacts Earbuds",
			Subtitle:    "纯净原音，静享心流时刻",
			Description: "智能主动降噪耳机 Artifacts Earbuds，拥有高达 45dB 的宽频深度降噪，配合专研声学单元，为您还原音乐细节。",
			MainImage:   "https://images.unsplash.com/photo-1505740420928-5e560c06d30e?auto=format&fit=crop&w=800&q=80",
			Images:      `["https://images.unsplash.com/photo-1505740420928-5e560c06d30e"]`,
			DetailHTML:  "<p>Artifacts Earbuds 详细介绍内容...</p>",
			Status:      1,
		},
		{
			ID:          4,
			CategoryID:  4,
			Name:        "Spike Pad Air",
			Subtitle:    "轻薄随行，创意触手可及",
			Description: "Spike Pad Air 只有 6.1 毫米的厚度，极佳的手写笔体验与灵敏的触控反馈，不管是画草图、记笔记还是浏览网页都能轻松胜任。",
			MainImage:   "https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?auto=format&fit=crop&w=800&q=80",
			Images:      `["https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0"]`,
			DetailHTML:  "<p>Spike Pad Air 详细介绍内容...</p>",
			Status:      1,
		},
	}
	for _, spu := range spus {
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&spu).Error; err != nil {
			return err
		}
	}

	for _, sku := range seedSkus() {
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&sku).Error; err != nil {
			return err
		}
	}
	return nil
}

func SeedInventoryData(db *gorm.DB) error {
	for _, sku := range seedSkus() {
		inventory := SkuInventory{SkuID: sku.ID, Available: sku.Stock}
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&inventory).Error; err != nil {
			return err
		}
	}
	return nil
}

func SeedPromotionData(db *gorm.DB, userID uint) error {
	if userID == 0 {
		userID = 1
	}

	now := time.Now()
	coupons := []Coupon{
		{ID: 1, Name: "10元无门槛券", Type: 3, Value: 1000, MinAmount: 0, StartTime: now, EndTime: now.AddDate(0, 1, 0)},
		{ID: 2, Name: "满500减50优惠券", Type: 1, Value: 5000, MinAmount: 50000, StartTime: now, EndTime: now.AddDate(0, 1, 0)},
		{ID: 3, Name: "满1000打9折优惠券", Type: 2, Value: 90, MinAmount: 100000, StartTime: now, EndTime: now.AddDate(0, 1, 0)},
		{ID: 4, Name: "满150减15券", Type: 1, Value: 1500, MinAmount: 15000, StartTime: now, EndTime: now.AddDate(0, 1, 0)},
	}
	for _, coupon := range coupons {
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&coupon).Error; err != nil {
			return err
		}
	}

	userCoupons := []UserCoupon{
		{UserID: userID, CouponID: 1, Status: UserCouponStatusAvailable},
		{UserID: userID, CouponID: 2, Status: UserCouponStatusAvailable},
		{UserID: userID, CouponID: 3, Status: UserCouponStatusAvailable},
	}
	for _, userCoupon := range userCoupons {
		if err := db.Where("user_id = ? AND coupon_id = ?", userCoupon.UserID, userCoupon.CouponID).
			FirstOrCreate(&userCoupon).Error; err != nil {
			return err
		}
	}
	return nil
}

func SeedAddressData(db *gorm.DB, userID uint) error {
	if userID == 0 {
		userID = 1
	}

	var count int64
	if err := db.Model(&Address{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	addresses := []Address{
		{
			UserID:        userID,
			ReceiverName:  "张小华",
			ReceiverPhone: "13800138000",
			Province:      "北京市",
			City:          "北京市",
			District:      "朝阳区",
			DetailAddress: "科创大厦 10 层 1001 室",
			IsDefault:     true,
		},
		{
			UserID:        userID,
			ReceiverName:  "李大明",
			ReceiverPhone: "13911112222",
			Province:      "上海市",
			City:          "上海市",
			District:      "浦东新区",
			DetailAddress: "世纪大道 88 号金茂大厦",
			IsDefault:     false,
		},
	}
	for _, address := range addresses {
		if err := db.Create(&address).Error; err != nil {
			return err
		}
	}
	return nil
}

func seedSkus() []Sku {
	return []Sku{
		{ID: 1, SpuID: 1, Title: "Haiku (128GB)", Specs: `{"规格": "128GB / 温暖沙丘"}`, Price: 39900, Stock: 87},
		{ID: 2, SpuID: 1, Title: "Sonnet (256GB)", Specs: `{"规格": "256GB / 珊瑚礁"}`, Price: 59900, Stock: 50},
		{ID: 3, SpuID: 1, Title: "Opus (512GB)", Specs: `{"规格": "512GB / 深邃星空"}`, Price: 89900, Stock: 20},
		{ID: 4, SpuID: 2, Title: "Haiku Core (16G+512G)", Specs: `{"规格": "16GB / 512GB SSD / 银色"}`, Price: 899900, Stock: 15},
		{ID: 5, SpuID: 2, Title: "Sonnet Core (32G+1T)", Specs: `{"规格": "32GB / 1TB SSD / 深空灰"}`, Price: 1299900, Stock: 10},
		{ID: 6, SpuID: 2, Title: "Opus Core (64G+2T)", Specs: `{"规格": "64GB / 2TB SSD / 珊瑚金"}`, Price: 1899900, Stock: 5},
		{ID: 7, SpuID: 3, Title: "Standard Edition", Specs: `{"规格": "标准版 / 象牙白"}`, Price: 99900, Stock: 100},
		{ID: 8, SpuID: 3, Title: "ANC Pro Edition", Specs: `{"规格": "降噪旗舰版 / 珊瑚红"}`, Price: 149900, Stock: 45},
		{ID: 9, SpuID: 4, Title: "WiFi (128GB)", Specs: `{"规格": "128GB / 经典灰"}`, Price: 459900, Stock: 30},
		{ID: 10, SpuID: 4, Title: "Cellular + WiFi (256GB)", Specs: `{"规格": "256GB / 蜂窝版 / 沙丘金"}`, Price: 559900, Stock: 12},
	}
}
