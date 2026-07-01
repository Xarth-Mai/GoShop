package checkout

import (
	"testing"

	"GoShop/models"
)

func TestCouponDiscount(t *testing.T) {
	tests := []struct {
		name     string
		coupon   models.Coupon
		subtotal int
		want     int
	}{
		{name: "full reduction", coupon: models.Coupon{Type: 1, Value: 5000}, subtotal: 50000, want: 5000},
		{name: "percentage", coupon: models.Coupon{Type: 2, Value: 90}, subtotal: 100000, want: 10000},
		{name: "cash coupon capped", coupon: models.Coupon{Type: 3, Value: 1000}, subtotal: 900, want: 900},
		{name: "unknown type", coupon: models.Coupon{Type: 99, Value: 1000}, subtotal: 900, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := couponDiscount(tt.coupon, tt.subtotal); got != tt.want {
				t.Fatalf("couponDiscount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAllocateDiscount(t *testing.T) {
	items := []ItemPreview{
		{SkuID: 1, Quantity: 1, OriginAmount: 20000},
		{SkuID: 2, Quantity: 1, OriginAmount: 10000},
	}

	allocateDiscount(items, 5000)

	if items[0].ItemDiscountAmount != 3333 {
		t.Fatalf("first item discount = %d, want 3333", items[0].ItemDiscountAmount)
	}
	if items[1].ItemDiscountAmount != 1667 {
		t.Fatalf("second item discount = %d, want 1667", items[1].ItemDiscountAmount)
	}
	if items[0].PayableAmount+items[1].PayableAmount != 25000 {
		t.Fatalf("payable total = %d, want 25000", items[0].PayableAmount+items[1].PayableAmount)
	}
}
