package checkout

import (
	"fmt"
	"time"

	"GoShop/models"

	"gorm.io/gorm"
)

type ItemReq struct {
	SkuID    uint `json:"skuId" binding:"required"`
	Quantity int  `json:"quantity" binding:"required"`
}

type PreviewRequest struct {
	Items        []ItemReq `json:"items" binding:"required,dive"`
	AddressID    uint      `json:"addressId" binding:"required"`
	UserCouponID uint      `json:"userCouponId"`
}

type ItemPreview struct {
	SkuID              uint `json:"skuId"`
	Quantity           int  `json:"quantity"`
	Price              int  `json:"price"`
	OriginAmount       int  `json:"originAmount"`
	ItemDiscountAmount int  `json:"itemDiscountAmount"`
	PayableAmount      int  `json:"payableAmount"`
}

type CouponCandidate struct {
	UserCouponID   uint   `json:"userCouponId"`
	Available      bool   `json:"available"`
	Reason         string `json:"reason"`
	DiscountAmount int    `json:"discountAmount"`
}

type Preview struct {
	Items                []ItemPreview     `json:"items"`
	GoodsOriginAmount    int               `json:"goodsOriginAmount"`
	GoodsDiscountAmount  int               `json:"goodsDiscountAmount"`
	ShippingFee          int               `json:"shippingFee"`
	TaxFee               int               `json:"taxFee"`
	PayableAmount        int               `json:"payableAmount"`
	SelectedUserCouponID uint              `json:"selectedUserCouponId"`
	CouponCandidates     []CouponCandidate `json:"couponCandidates"`
}

type Service struct {
	DB *gorm.DB
}

func NewService(db *gorm.DB) Service {
	return Service{DB: db}
}

func (s Service) Calculate(userID uint, req PreviewRequest) (Preview, error) {
	if len(req.Items) == 0 {
		return Preview{}, fmt.Errorf("下单商品清单不能为空")
	}
	if req.AddressID > 0 {
		var address models.Address
		if err := s.DB.Where("id = ? AND user_id = ?", req.AddressID, userID).First(&address).Error; err != nil {
			return Preview{}, fmt.Errorf("收货地址不存在")
		}
	}

	var preview Preview
	for _, item := range req.Items {
		if item.Quantity <= 0 {
			return Preview{}, fmt.Errorf("商品数量必须大于 0")
		}
		var sku models.Sku
		if err := s.DB.Where("id = ?", item.SkuID).First(&sku).Error; err != nil {
			return Preview{}, fmt.Errorf("商品规格 ID %d 不存在", item.SkuID)
		}
		origin := sku.Price * item.Quantity
		preview.Items = append(preview.Items, ItemPreview{
			SkuID:         item.SkuID,
			Quantity:      item.Quantity,
			Price:         sku.Price,
			OriginAmount:  origin,
			PayableAmount: origin,
		})
		preview.GoodsOriginAmount += origin
	}

	preview.ShippingFee = 1000
	if preview.GoodsOriginAmount >= 9900 {
		preview.ShippingFee = 0
	}
	preview.TaxFee = preview.GoodsOriginAmount * 5 / 100

	preview.CouponCandidates = s.couponCandidates(userID, req.UserCouponID, preview.GoodsOriginAmount)
	selectedDiscount := 0
	for _, candidate := range preview.CouponCandidates {
		if candidate.UserCouponID == req.UserCouponID && candidate.Available {
			selectedDiscount = candidate.DiscountAmount
			preview.SelectedUserCouponID = candidate.UserCouponID
			break
		}
	}
	if selectedDiscount > preview.GoodsOriginAmount {
		selectedDiscount = preview.GoodsOriginAmount
	}
	preview.GoodsDiscountAmount = selectedDiscount
	allocateDiscount(preview.Items, selectedDiscount)

	preview.PayableAmount = preview.GoodsOriginAmount + preview.ShippingFee + preview.TaxFee - preview.GoodsDiscountAmount
	if preview.GoodsOriginAmount > 0 && preview.PayableAmount <= 0 {
		preview.PayableAmount = 1
	}

	return preview, nil
}

func (s Service) couponCandidates(userID, selectedUserCouponID uint, subtotal int) []CouponCandidate {
	now := time.Now()
	query := s.DB.Preload("Coupon").
		Joins("JOIN coupons ON coupons.id = user_coupons.coupon_id").
		Where("user_coupons.user_id = ? AND user_coupons.status = ? AND coupons.end_time >= ?", userID, 0, now)
	if selectedUserCouponID > 0 {
		query = query.Or("user_coupons.id = ? AND user_coupons.user_id = ?", selectedUserCouponID, userID)
	}

	var userCoupons []models.UserCoupon
	if err := query.Find(&userCoupons).Error; err != nil {
		return nil
	}

	candidates := make([]CouponCandidate, 0, len(userCoupons))
	for _, userCoupon := range userCoupons {
		candidate := CouponCandidate{UserCouponID: userCoupon.ID, Available: true}
		if userCoupon.Status != 0 {
			candidate.Available = false
			candidate.Reason = "优惠券已使用或已失效"
		} else if userCoupon.Coupon.StartTime.After(now) || userCoupon.Coupon.EndTime.Before(now) {
			candidate.Available = false
			candidate.Reason = "优惠券不在有效期内"
		} else if subtotal < userCoupon.Coupon.MinAmount {
			candidate.Available = false
			candidate.Reason = fmt.Sprintf("未达到满 %d 元使用门槛", userCoupon.Coupon.MinAmount/100)
		} else {
			candidate.DiscountAmount = couponDiscount(userCoupon.Coupon, subtotal)
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}

func couponDiscount(coupon models.Coupon, subtotal int) int {
	discount := 0
	switch coupon.Type {
	case 1, 3:
		discount = coupon.Value
	case 2:
		discount = subtotal * (100 - coupon.Value) / 100
	}
	if discount > subtotal {
		return subtotal
	}
	if discount < 0 {
		return 0
	}
	return discount
}

func allocateDiscount(items []ItemPreview, discount int) {
	if discount <= 0 || len(items) == 0 {
		for i := range items {
			items[i].ItemDiscountAmount = 0
			items[i].PayableAmount = items[i].OriginAmount
		}
		return
	}

	totalEligible := 0
	for _, item := range items {
		totalEligible += item.OriginAmount
	}
	if totalEligible <= 0 {
		return
	}

	remain := discount
	for i := range items {
		alloc := 0
		if i == len(items)-1 {
			alloc = remain
		} else {
			alloc = discount * items[i].OriginAmount / totalEligible
		}
		if alloc > items[i].OriginAmount {
			alloc = items[i].OriginAmount
		}
		if alloc < 0 {
			alloc = 0
		}
		items[i].ItemDiscountAmount = alloc
		items[i].PayableAmount = items[i].OriginAmount - alloc
		remain -= alloc
	}
}
