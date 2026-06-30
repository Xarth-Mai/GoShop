package models

import (
	"time"

	"gorm.io/gorm"
)

// Category 商品分类
type Category struct {
	ID        uint       `gorm:"primaryKey;column:id;autoIncrement" json:"id"`
	ParentID  uint       `gorm:"column:parent_id;default:0" json:"parentId"`
	Name      string     `gorm:"column:name;type:varchar(64);not null" json:"name"`
	SortOrder int        `gorm:"column:sort_order;default:0" json:"sortOrder"`
	CreatedAt time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt time.Time  `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

// Spu 标准产品单元
type Spu struct {
	ID          uint       `gorm:"primaryKey;column:id;autoIncrement" json:"id"`
	CategoryID  uint       `gorm:"column:category_id;not null" json:"categoryId"`
	Name        string     `gorm:"column:name;type:varchar(128);not null" json:"name"`
	Subtitle    string     `gorm:"column:subtitle;type:varchar(256)" json:"subtitle"`
	Description string     `gorm:"column:description;type:text" json:"description"`
	MainImage   string     `gorm:"column:main_image;type:varchar(512)" json:"mainImage"`
	Images      string     `gorm:"column:images;type:jsonb" json:"images"` // 存储图片JSON数组
	DetailHTML  string     `gorm:"column:detail_html;type:text" json:"detailHtml"`
	Status      int        `gorm:"column:status;default:1" json:"status"` // 1: 上架, 2: 下架
	CreatedAt   time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
	Skus        []Sku      `gorm:"foreignKey:SpuID" json:"skus,omitempty"`
}

// Sku 库存量单位
type Sku struct {
	ID          uint       `gorm:"primaryKey;column:id;autoIncrement" json:"id"`
	SpuID       uint       `gorm:"column:spu_id;not null" json:"spuId"`
	Title       string     `gorm:"column:title;type:varchar(256);not null" json:"title"`
	Specs       string     `gorm:"column:specs;type:jsonb" json:"specs"` // 存储规格JSON属性
	Price       int        `gorm:"column:price;not null" json:"price"`   // 单位：分
	Stock       int        `gorm:"column:stock;default:0;not null" json:"stock"`
	SalesVolume int        `gorm:"column:sales_volume;default:0" json:"salesVolume"`
	CreatedAt   time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

// SeedProducts 初始化商品测试数据
func SeedProducts(db *gorm.DB) error {
	var count int64
	db.Model(&Category{}).Count(&count)
	if count > 0 {
		return nil // 已经有数据，不再重复插入
	}

	// 1. 插入分类
	categories := []Category{
		{ID: 1, Name: "智能手机", SortOrder: 1},
		{ID: 2, Name: "笔记本", SortOrder: 2},
		{ID: 3, Name: "穿戴数码", SortOrder: 3},
		{ID: 4, Name: "智能平板", SortOrder: 4},
	}
	for _, c := range categories {
		if err := db.Create(&c).Error; err != nil {
			return err
		}
	}

	// 2. 插入 SPU
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
	for _, s := range spus {
		if err := db.Create(&s).Error; err != nil {
			return err
		}
	}

	// 3. 插入 SKU
	skus := []Sku{
		// SPU 1: Claude Phone 1
		{ID: 1, SpuID: 1, Title: "Haiku (128GB)", Specs: `{"规格": "128GB / 温暖沙丘"}`, Price: 39900, Stock: 87},
		{ID: 2, SpuID: 1, Title: "Sonnet (256GB)", Specs: `{"规格": "256GB / 珊瑚礁"}`, Price: 59900, Stock: 50},
		{ID: 3, SpuID: 1, Title: "Opus (512GB)", Specs: `{"规格": "512GB / 深邃星空"}`, Price: 89900, Stock: 20},

		// SPU 2: Anthropic Book Pro
		{ID: 4, SpuID: 2, Title: "Haiku Core (16G+512G)", Specs: `{"规格": "16GB / 512GB SSD / 银色"}`, Price: 899900, Stock: 15},
		{ID: 5, SpuID: 2, Title: "Sonnet Core (32G+1T)", Specs: `{"规格": "32GB / 1TB SSD / 深空灰"}`, Price: 1299900, Stock: 10},
		{ID: 6, SpuID: 2, Title: "Opus Core (64G+2T)", Specs: `{"规格": "64GB / 2TB SSD / 珊瑚金"}`, Price: 1899900, Stock: 5},

		// SPU 3: Artifacts Earbuds
		{ID: 7, SpuID: 3, Title: "Standard Edition", Specs: `{"规格": "标准版 / 象牙白"}`, Price: 99900, Stock: 100},
		{ID: 8, SpuID: 3, Title: "ANC Pro Edition", Specs: `{"规格": "降噪旗舰版 / 珊瑚红"}`, Price: 149900, Stock: 45},

		// SPU 4: Spike Pad Air
		{ID: 9, SpuID: 4, Title: "WiFi (128GB)", Specs: `{"规格": "128GB / 经典灰"}`, Price: 459900, Stock: 30},
		{ID: 10, SpuID: 4, Title: "Cellular + WiFi (256GB)", Specs: `{"规格": "256GB / 蜂窝版 / 沙丘金"}`, Price: 559900, Stock: 12},
	}
	for _, s := range skus {
		if err := db.Create(&s).Error; err != nil {
			return err
		}
	}

	return nil
}
