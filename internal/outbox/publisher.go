package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"GoShop/core"
	"GoShop/models"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Publisher struct {
	DB     *gorm.DB
	Logger *zap.Logger
}

func NewPublisher(db *gorm.DB, logger *zap.Logger) Publisher {
	return Publisher{DB: db, Logger: logger}
}

func (p Publisher) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.PublishPending(ctx, 100); err != nil && p.Logger != nil {
				p.Logger.Warn("outbox publish tick failed", zap.Error(err))
			}
		}
	}
}

func (p Publisher) PublishPending(ctx context.Context, limit int) error {
	if p.DB == nil {
		return nil
	}
	if limit <= 0 {
		limit = 100
	}

	var events []models.OutboxEvent
	if err := p.DB.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("status = ? AND next_retry_at <= ?", models.OutboxStatusPending, time.Now()).
		Order("id asc").
		Limit(limit).
		Find(&events).Error; err != nil {
		return err
	}

	for _, event := range events {
		if err := p.publishOne(ctx, event); err != nil {
			if p.Logger != nil {
				p.Logger.Warn("outbox event publish failed", zap.String("event_id", event.EventID), zap.Error(err))
			}
			if updateErr := p.markFailed(event.ID); updateErr != nil {
				return updateErr
			}
			continue
		}
		if err := p.markSent(event.ID); err != nil {
			return err
		}
	}
	return nil
}

func (p Publisher) publishOne(ctx context.Context, event models.OutboxEvent) error {
	if core.JetStream == nil {
		return errors.New("nats: no connection")
	}

	// 拼装 Subject，如 goshop.events.order.created
	subject := fmt.Sprintf("goshop.events.%s.%s", strings.ToLower(event.AggregateType), strings.ToLower(event.EventType))

	msgData, err := json.Marshal(map[string]interface{}{
		"event_id":       event.EventID,
		"aggregate_type": event.AggregateType,
		"aggregate_id":   event.AggregateID,
		"event_type":     event.EventType,
		"payload":        event.Payload,
		"created_at":     event.CreatedAt,
	})
	if err != nil {
		return err
	}

	_, err = core.JetStream.Publish(subject, msgData)
	return err
}

func (p Publisher) markSent(id uint) error {
	now := time.Now()
	return p.DB.Model(&models.OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     models.OutboxStatusSent,
			"updated_at": now,
		}).Error
}

func (p Publisher) markFailed(id uint) error {
	now := time.Now()
	return p.DB.Model(&models.OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"retry_count":   gorm.Expr("retry_count + 1"),
			"next_retry_at": now.Add(30 * time.Second),
			"updated_at":    now,
		}).Error
}
