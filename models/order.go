package models

import (
	"time"
)

const (
	OrderStatusPendingPayment = 10
	OrderStatusPaid           = 20
	OrderStatusCanceled       = 60
	OrderStatusRefundApplying = 110
	OrderStatusRefunded       = 120
	OrderStatusRefundRejected = 130

	PayStatusUnpaid          = 0
	PayStatusPaid            = 20
	PayStatusPartialRefunded = 30
	PayStatusRefunded        = 40

	AfterSaleStatusNone     = 0
	AfterSaleStatusApplying = 10
	AfterSaleStatusRejected = 30
	AfterSaleStatusRefunded = 70

	PaymentChannelMock = 4

	PaymentStatusCreated = 10
	PaymentStatusPaid    = 30
	PaymentStatusFailed  = 50

	RefundStatusCreated = 10
	RefundStatusSuccess = 30

	TransactionStatusProcessed = 20
	TransactionStatusFailed    = 30

	AccountingDirectionDebit  = 1
	AccountingDirectionCredit = 2
)

// Order 升级后的订单主表模型
type Order struct {
	ID                     string     `gorm:"primaryKey;column:id;type:varchar(64)" json:"id"`
	UserID                 uint       `gorm:"column:user_id;not null;index" json:"userId"`
	TotalAmount            int        `gorm:"column:total_amount;not null" json:"totalAmount"`                 // 兼容旧前端：应付总额（分）
	DiscountAmount         int        `gorm:"column:discount_amount;default:0;not null" json:"discountAmount"` // 兼容旧前端：商品优惠总额（分）
	GoodsOriginAmount      int        `gorm:"column:goods_origin_amount;default:0;not null" json:"goodsOriginAmount"`
	GoodsDiscountAmount    int        `gorm:"column:goods_discount_amount;default:0;not null" json:"goodsDiscountAmount"`
	ShippingFee            int        `gorm:"column:shipping_fee;default:0;not null" json:"shippingFee"` // 运费（分）
	ShippingDiscountAmount int        `gorm:"column:shipping_discount_amount;default:0;not null" json:"shippingDiscountAmount"`
	TaxFee                 int        `gorm:"column:tax_fee;default:0;not null" json:"taxFee"` // 税费（分）
	TaxDiscountAmount      int        `gorm:"column:tax_discount_amount;default:0;not null" json:"taxDiscountAmount"`
	PayableAmount          int        `gorm:"column:payable_amount;default:0;not null" json:"payableAmount"`
	Status                 int        `gorm:"column:status;default:10;not null" json:"status"` // 10: 待支付, 20: 已支付, 60: 已取消, 110: 申请退款中, 120: 已退款, 130: 退款被拒绝
	PayStatus              int        `gorm:"column:pay_status;default:0;not null" json:"payStatus"`
	AfterSaleStatus        int        `gorm:"column:after_sale_status;default:0;not null" json:"afterSaleStatus"`
	UserCouponID           uint       `gorm:"column:user_coupon_id;default:0;not null" json:"userCouponId"`
	RefundReason           string     `gorm:"column:refund_reason;type:varchar(256)" json:"refundReason"`
	RefundProof            string     `gorm:"column:refund_proof;type:varchar(512)" json:"refundProof"`
	ReceiverName           string     `gorm:"column:receiver_name;type:varchar(256)" json:"receiverName"`   // 收货人姓名快照
	ReceiverPhone          string     `gorm:"column:receiver_phone;type:varchar(256)" json:"receiverPhone"` // 收货人手机快照
	ReceiverAddr           string     `gorm:"column:receiver_addr;type:varchar(512)" json:"receiverAddr"`   // 收货地址快照
	PayExpireAt            *time.Time `gorm:"column:pay_expire_at" json:"payExpireAt,omitempty"`
	CreatedAt              time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt              time.Time  `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`

	// 订单子项
	Items []OrderItem `gorm:"foreignKey:OrderID" json:"items"`
}

// OrderItem 订单商品子项模型
type OrderItem struct {
	ID                 uint      `gorm:"primaryKey;column:id;autoIncrement" json:"id"`
	OrderID            string    `gorm:"column:order_id;type:varchar(64);not null;index" json:"orderId"`
	SkuID              uint      `gorm:"column:sku_id;not null" json:"skuId"`
	Price              int       `gorm:"column:price;not null" json:"price"` // 下单单价（分）
	Quantity           int       `gorm:"column:quantity;not null" json:"quantity"`
	OriginAmount       int       `gorm:"column:origin_amount;default:0;not null" json:"originAmount"`
	ItemDiscountAmount int       `gorm:"column:item_discount_amount;default:0;not null" json:"itemDiscountAmount"`
	PayableAmount      int       `gorm:"column:payable_amount;default:0;not null" json:"payableAmount"`
	RefundedAmount     int       `gorm:"column:refunded_amount;default:0;not null" json:"refundedAmount"`
	MerchantID         uint      `gorm:"column:merchant_id;default:0;not null" json:"merchantId"`
	PromotionSnapshot  string    `gorm:"column:promotion_snapshot;type:text" json:"promotionSnapshot"`
	CreatedAt          time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt          time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`

	// 关联 SKU
	Sku Sku `gorm:"foreignKey:SkuID" json:"sku"`
}

// OrderPromotionAllocation 保存优惠分摊结果，支撑部分退款和对账解释。
type OrderPromotionAllocation struct {
	ID                 uint      `gorm:"primaryKey;column:id;autoIncrement" json:"id"`
	OrderID            string    `gorm:"column:order_id;type:varchar(64);not null;index" json:"orderId"`
	OrderItemID        uint      `gorm:"column:order_item_id;index" json:"orderItemId"`
	SkuID              uint      `gorm:"column:sku_id;not null;index" json:"skuId"`
	CampaignID         uint      `gorm:"column:campaign_id;default:0;not null" json:"campaignId"`
	UserCouponID       uint      `gorm:"column:user_coupon_id;default:0;not null;index" json:"userCouponId"`
	DiscountType       int       `gorm:"column:discount_type;not null" json:"discountType"`
	DiscountAmount     int       `gorm:"column:discount_amount;not null" json:"discountAmount"`
	AllocationSnapshot string    `gorm:"column:allocation_snapshot;type:text" json:"allocationSnapshot"`
	CreatedAt          time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
}

// OrderStateLog 记录订单状态流转。
type OrderStateLog struct {
	ID           uint      `gorm:"primaryKey;column:id;autoIncrement" json:"id"`
	OrderID      string    `gorm:"column:order_id;type:varchar(64);not null;index" json:"orderId"`
	FromStatus   int       `gorm:"column:from_status" json:"fromStatus"`
	ToStatus     int       `gorm:"column:to_status;not null" json:"toStatus"`
	OperatorType int       `gorm:"column:operator_type;not null" json:"operatorType"`
	OperatorID   uint      `gorm:"column:operator_id" json:"operatorId"`
	Event        string    `gorm:"column:event;type:varchar(64);not null" json:"event"`
	Remark       string    `gorm:"column:remark;type:text" json:"remark"`
	CreatedAt    time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
}

type PaymentOrder struct {
	ID             string     `gorm:"primaryKey;column:id;type:varchar(64)" json:"id"`
	OrderID        string     `gorm:"column:order_id;type:varchar(64);not null;index" json:"orderId"`
	UserID         uint       `gorm:"column:user_id;not null;index" json:"userId"`
	Channel        int        `gorm:"column:channel;not null" json:"channel"`
	Amount         int        `gorm:"column:amount;not null" json:"amount"`
	Currency       string     `gorm:"column:currency;type:varchar(8);default:'CNY';not null" json:"currency"`
	Status         int        `gorm:"column:status;not null;index" json:"status"`
	ChannelTradeNo string     `gorm:"column:channel_trade_no;type:varchar(128)" json:"channelTradeNo"`
	PayURL         string     `gorm:"column:pay_url;type:text" json:"payUrl"`
	IdempotencyKey string     `gorm:"column:idempotency_key;type:varchar(128);uniqueIndex" json:"idempotencyKey"`
	PaidAt         *time.Time `gorm:"column:paid_at" json:"paidAt,omitempty"`
	Version        int        `gorm:"column:version;default:0;not null" json:"version"`
	CreatedAt      time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

type PaymentTransaction struct {
	ID             uint      `gorm:"primaryKey;column:id;autoIncrement" json:"id"`
	PaymentOrderID string    `gorm:"column:payment_order_id;type:varchar(64);not null;index" json:"paymentOrderId"`
	Channel        int       `gorm:"column:channel;not null;uniqueIndex:idx_payment_channel_event" json:"channel"`
	ChannelEventID string    `gorm:"column:channel_event_id;type:varchar(128);not null;uniqueIndex:idx_payment_channel_event" json:"channelEventId"`
	EventType      string    `gorm:"column:event_type;type:varchar(64);not null" json:"eventType"`
	RawPayload     string    `gorm:"column:raw_payload;type:text;not null" json:"rawPayload"`
	Signature      string    `gorm:"column:signature;type:varchar(512)" json:"signature"`
	ProcessStatus  int       `gorm:"column:process_status;not null" json:"processStatus"`
	ErrorMessage   string    `gorm:"column:error_message;type:text" json:"errorMessage"`
	CreatedAt      time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
}

type RefundOrder struct {
	ID              string     `gorm:"primaryKey;column:id;type:varchar(64)" json:"id"`
	PaymentOrderID  string     `gorm:"column:payment_order_id;type:varchar(64);not null;index" json:"paymentOrderId"`
	OrderID         string     `gorm:"column:order_id;type:varchar(64);not null;index" json:"orderId"`
	AfterSaleID     string     `gorm:"column:after_sale_id;type:varchar(64)" json:"afterSaleId"`
	Amount          int        `gorm:"column:amount;not null" json:"amount"`
	Reason          string     `gorm:"column:reason;type:varchar(256)" json:"reason"`
	Status          int        `gorm:"column:status;not null" json:"status"`
	ChannelRefundNo string     `gorm:"column:channel_refund_no;type:varchar(128)" json:"channelRefundNo"`
	IdempotencyKey  string     `gorm:"column:idempotency_key;type:varchar(128);uniqueIndex" json:"idempotencyKey"`
	RefundedAt      *time.Time `gorm:"column:refunded_at" json:"refundedAt,omitempty"`
	CreatedAt       time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

type AccountingEntry struct {
	ID          uint      `gorm:"primaryKey;column:id;autoIncrement" json:"id"`
	BizType     string    `gorm:"column:biz_type;type:varchar(64);not null;uniqueIndex:idx_accounting_entry" json:"bizType"`
	BizID       string    `gorm:"column:biz_id;type:varchar(128);not null;uniqueIndex:idx_accounting_entry" json:"bizId"`
	AccountType string    `gorm:"column:account_type;type:varchar(64);not null;uniqueIndex:idx_accounting_entry" json:"accountType"`
	Direction   int       `gorm:"column:direction;not null;uniqueIndex:idx_accounting_entry" json:"direction"`
	Amount      int       `gorm:"column:amount;not null" json:"amount"`
	Currency    string    `gorm:"column:currency;type:varchar(8);default:'CNY';not null" json:"currency"`
	CreatedAt   time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
}

type AfterSaleOrder struct {
	ID               string          `gorm:"primaryKey;column:id;type:varchar(64)" json:"id"`
	OrderID          string          `gorm:"column:order_id;type:varchar(64);not null;index" json:"orderId"`
	UserID           uint            `gorm:"column:user_id;not null;index" json:"userId"`
	Type             int             `gorm:"column:type;not null" json:"type"`
	Status           int             `gorm:"column:status;not null" json:"status"`
	Reason           string          `gorm:"column:reason;type:varchar(256)" json:"reason"`
	ProofURLs        string          `gorm:"column:proof_urls;type:text" json:"proofUrls"`
	ApplyAmount      int             `gorm:"column:apply_amount;not null" json:"applyAmount"`
	ApprovedAmount   int             `gorm:"column:approved_amount;default:0;not null" json:"approvedAmount"`
	RefundID         string          `gorm:"column:refund_id;type:varchar(64)" json:"refundId"`
	ReturnTrackingNo string          `gorm:"column:return_tracking_no;type:varchar(128)" json:"returnTrackingNo"`
	Version          int             `gorm:"column:version;default:0;not null" json:"version"`
	CreatedAt        time.Time       `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt        time.Time       `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
	Items            []AfterSaleItem `gorm:"foreignKey:AfterSaleID" json:"items"`
}

type AfterSaleItem struct {
	ID                  uint      `gorm:"primaryKey;column:id;autoIncrement" json:"id"`
	AfterSaleID         string    `gorm:"column:after_sale_id;type:varchar(64);not null;index" json:"afterSaleId"`
	OrderItemID         uint      `gorm:"column:order_item_id;not null;index" json:"orderItemId"`
	SkuID               uint      `gorm:"column:sku_id;not null" json:"skuId"`
	Quantity            int       `gorm:"column:quantity;not null" json:"quantity"`
	MaxRefundableAmount int       `gorm:"column:max_refundable_amount;not null" json:"maxRefundableAmount"`
	ApplyAmount         int       `gorm:"column:apply_amount;not null" json:"applyAmount"`
	ApprovedAmount      int       `gorm:"column:approved_amount;default:0;not null" json:"approvedAmount"`
	CreatedAt           time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
}

// DeadLetterOrder 延迟队列死信订单表 (DLQ)
type DeadLetterOrder struct {
	ID        uint      `gorm:"primaryKey;column:id;autoIncrement" json:"id"`
	OrderID   string    `gorm:"column:order_id;type:varchar(64);not null;uniqueIndex" json:"orderId"`
	Reason    string    `gorm:"column:reason;type:text" json:"reason"`
	CreatedAt time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
}
