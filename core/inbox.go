package core

import (
	"time"

	"GoShop/models"

	"gorm.io/gorm"
)

// ProcessWithInbox 事务级消息幂等去重包装器
// 1. 查询本地数据库的 inbox_events 是否已经处理过该消息
// 2. 若未处理过，执行业务逻辑回调 task
// 3. 将消息 ID 记录入库以作去重标记，最后提交事务
func ProcessWithInbox(tx *gorm.DB, eventID string, consumerName string, task func(tx *gorm.DB) error) error {
	var count int64
	err := tx.Model(&models.InboxEvent{}).Where("event_id = ?", eventID).Count(&count).Error
	if err != nil {
		return err
	}
	if count > 0 {
		// 已经处理过，直接返回成功，实现幂等幂等防重消费
		return nil
	}

	// 执行真正的业务处理
	if err := task(tx); err != nil {
		return err
	}

	// 记录事件标记
	inbox := models.InboxEvent{
		EventID:      eventID,
		ConsumerName: consumerName,
		ProcessedAt:  time.Now(),
	}
	return tx.Create(&inbox).Error
}
