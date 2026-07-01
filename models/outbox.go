package models

import "time"

const (
	OutboxStatusPending = 0
	OutboxStatusSent    = 1
	OutboxStatusFailed  = 2
)

type OutboxEvent struct {
	ID            uint      `gorm:"primaryKey;column:id;autoIncrement" json:"id"`
	EventID       string    `gorm:"column:event_id;type:varchar(128);not null;uniqueIndex" json:"eventId"`
	AggregateType string    `gorm:"column:aggregate_type;type:varchar(64);not null;index" json:"aggregateType"`
	AggregateID   string    `gorm:"column:aggregate_id;type:varchar(128);not null;index" json:"aggregateId"`
	EventType     string    `gorm:"column:event_type;type:varchar(64);not null;index" json:"eventType"`
	Payload       string    `gorm:"column:payload;type:text;not null" json:"payload"`
	Status        int       `gorm:"column:status;default:0;not null;index" json:"status"`
	RetryCount    int       `gorm:"column:retry_count;default:0;not null" json:"retryCount"`
	NextRetryAt   time.Time `gorm:"column:next_retry_at;default:CURRENT_TIMESTAMP;index" json:"nextRetryAt"`
	CreatedAt     time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt     time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

type InboxEvent struct {
	EventID      string    `gorm:"primaryKey;column:event_id;type:varchar(128)" json:"eventId"`
	ConsumerName string    `gorm:"column:consumer_name;type:varchar(128);not null;index" json:"consumerName"`
	ProcessedAt  time.Time `gorm:"column:processed_at;default:CURRENT_TIMESTAMP" json:"processedAt"`
}
