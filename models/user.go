package models

import (
	"time"
)

// User 用户模型
type User struct {
	ID           uint      `gorm:"primaryKey;column:id;autoIncrement" json:"id"`
	Username     string    `gorm:"column:username;unique;type:varchar(64);not null" json:"username"`
	PasswordHash string    `gorm:"column:password_hash;type:varchar(256);not null" json:"-"`
	Email        string    `gorm:"column:email;type:varchar(128)" json:"email"`
	CreatedAt    time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt    time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}
