package checkout

import (
	"fmt"

	"GoShop/internal/promotion"
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
	promotionCandidates := promotion.NewService(s.DB).CouponCandidates(userID, selectedUserCouponID, subtotal)
	candidates := make([]CouponCandidate, 0, len(promotionCandidates))
	for _, candidate := range promotionCandidates {
		candidates = append(candidates, CouponCandidate{
			UserCouponID:   candidate.UserCouponID,
			Available:      candidate.Available,
			Reason:         candidate.Reason,
			DiscountAmount: candidate.DiscountAmount,
		})
	}
	return candidates
}

func couponDiscount(coupon models.Coupon, subtotal int) int {
	return promotion.CouponDiscount(coupon, subtotal)
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
