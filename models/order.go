package models

import (
	"time"
)

// Order 升级后的订单主表模型
type Order struct {
	ID             string      `gorm:"primaryKey;column:id;type:varchar(64)" json:"id"`
	UserID         uint        `gorm:"column:user_id;not null;index" json:"userId"`
	TotalAmount    int         `gorm:"column:total_amount;not null" json:"totalAmount"`       // 应付总额（分）
	DiscountAmount int         `gorm:"column:discount_amount;default:0;not null" json:"discountAmount"` // 优惠券折扣金额（分）
	ShippingFee    int         `gorm:"column:shipping_fee;default:0;not null" json:"shippingFee"`       // 运费（分）
	TaxFee         int         `gorm:"column:tax_fee;default:0;not null" json:"taxFee"`                 // 税费（分）
	Status         int         `gorm:"column:status;default:1;not null" json:"status"`        // 1: 待支付, 2: 已支付, 3: 已取消, 4: 申请退款中, 5: 已退款, 6: 退款被拒绝
	RefundReason   string      `gorm:"column:refund_reason;type:varchar(256)" json:"refundReason"`
	RefundProof    string      `gorm:"column:refund_proof;type:varchar(512)" json:"refundProof"`
	ReceiverName   string      `gorm:"column:receiver_name;type:varchar(256)" json:"receiverName"`   // 收货人姓名快照
	ReceiverPhone  string      `gorm:"column:receiver_phone;type:varchar(256)" json:"receiverPhone"` // 收货人手机快照
	ReceiverAddr   string      `gorm:"column:receiver_addr;type:varchar(512)" json:"receiverAddr"`   // 收货地址快照
	CreatedAt      time.Time   `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt      time.Time   `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
	
	// 订单子项
	Items          []OrderItem `gorm:"foreignKey:OrderID" json:"items"`
}

// OrderItem 订单商品子项模型
type OrderItem struct {
	ID        uint      `gorm:"primaryKey;column:id;autoIncrement" json:"id"`
	OrderID   string    `gorm:"column:order_id;type:varchar(64);not null;index" json:"orderId"`
	SkuID     uint      `gorm:"column:sku_id;not null" json:"skuId"`
	Price     int       `gorm:"column:price;not null" json:"price"` // 下单单价（分）
	Quantity  int       `gorm:"column:quantity;not null" json:"quantity"`
	CreatedAt time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`

	// 关联 SKU
	Sku       Sku       `gorm:"foreignKey:SkuID" json:"sku"`
}

// DeadLetterOrder 延迟队列死信订单表 (DLQ)
type DeadLetterOrder struct {
	ID        uint      `gorm:"primaryKey;column:id;autoIncrement" json:"id"`
	OrderID   string    `gorm:"column:order_id;type:varchar(64);not null;uniqueIndex" json:"orderId"`
	Reason    string    `gorm:"column:reason;type:text" json:"reason"`
	CreatedAt time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
}
