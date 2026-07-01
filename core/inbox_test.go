package core_test

import (
	"errors"
	"testing"

	"GoShop/core"
	"GoShop/internal/testutil"
	"GoShop/models"

	"gorm.io/gorm"
)

func TestProcessWithInbox(t *testing.T) {
	db := testutil.SetupTestDB(t)

	eventID := "evt-test-1001"
	consumerName := "test-consumer"

	// 1. 首次消费，应该成功并写入 inbox_events
	err := db.Transaction(func(tx *gorm.DB) error {
		return core.ProcessWithInbox(tx, eventID, consumerName, func(dbTx *gorm.DB) error {
			// 执行一个简单的业务操作
			return dbTx.Create(&models.User{Username: "inbox_user_1", PasswordHash: "123"}).Error
		})
	})
	if err != nil {
		t.Fatalf("first process failed: %v", err)
	}

	// 验证 inbox_events 已有记录，且用户已创建
	var count int64
	db.Model(&models.InboxEvent{}).Where("event_id = ?", eventID).Count(&count)
	if count != 1 {
		t.Errorf("expected inbox_event count 1, got %d", count)
	}

	var userCount int64
	db.Model(&models.User{}).Where("username = ?", "inbox_user_1").Count(&userCount)
	if userCount != 1 {
		t.Errorf("expected user count 1, got %d", userCount)
	}

	// 2. 第二次消费相同事件，应该直接幂等返回成功，且不重复执行业务
	err = db.Transaction(func(tx *gorm.DB) error {
		return core.ProcessWithInbox(tx, eventID, consumerName, func(dbTx *gorm.DB) error {
			t.Error("duplicate event was executed, business callback should be skipped!")
			return nil
		})
	})
	if err != nil {
		t.Fatalf("second process failed: %v", err)
	}

	// 3. 业务回调失败，事务回滚且不插入 inbox_events
	failedEventID := "evt-test-failed"
	err = db.Transaction(func(tx *gorm.DB) error {
		return core.ProcessWithInbox(tx, failedEventID, consumerName, func(dbTx *gorm.DB) error {
			return errors.New("simulated error")
		})
	})
	if err == nil {
		t.Error("expected error, got nil")
	}

	db.Model(&models.InboxEvent{}).Where("event_id = ?", failedEventID).Count(&count)
	if count != 0 {
		t.Errorf("expected no inbox_event for failed task, got %d", count)
	}
}
