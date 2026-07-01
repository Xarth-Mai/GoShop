package outbox

import (
	"testing"

	"GoShop/internal/testutil"
	"GoShop/models"

	"gorm.io/gorm"
)

func TestPublishIdempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewService()

	for i := 0; i < 2; i++ {
		if err := db.Transaction(func(tx *gorm.DB) error {
			return svc.Publish(tx, "order", "ORDER-OUTBOX-1", "OrderCreated", map[string]interface{}{
				"orderId": "ORDER-OUTBOX-1",
			})
		}); err != nil {
			t.Fatalf("publish failed: %v", err)
		}
	}

	var count int64
	if err := db.Model(&models.OutboxEvent{}).Where("event_type = ? AND aggregate_id = ?", "OrderCreated", "ORDER-OUTBOX-1").Count(&count).Error; err != nil {
		t.Fatalf("count outbox events: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected idempotent single event, got %d", count)
	}
}
