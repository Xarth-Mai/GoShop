package outbox

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"GoShop/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct{}

func NewService() Service {
	return Service{}
}

func (s Service) Publish(tx *gorm.DB, aggregateType, aggregateID, eventType string, payload any) error {
	if aggregateType == "" || aggregateID == "" || eventType == "" {
		return fmt.Errorf("outbox event requires aggregate and event type")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	now := time.Now()
	event := models.OutboxEvent{
		EventID:       eventID(eventType, aggregateID),
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		Payload:       string(raw),
		Status:        models.OutboxStatusPending,
		NextRetryAt:   now,
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&event).Error
}

func eventID(eventType, aggregateID string) string {
	value := strings.ReplaceAll(eventType+":"+aggregateID, " ", "_")
	if len(value) <= 128 {
		return value
	}
	return value[:128]
}
