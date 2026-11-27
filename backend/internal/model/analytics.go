package model

import (
	"time"

	"gorm.io/datatypes"
)

type MiniappAnalyticsEvent struct {
	ID           uint64 `gorm:"primaryKey"`
	MerchantID   uint64 `gorm:"not null;uniqueIndex:uk_analytics_merchant_event,priority:1;index:idx_analytics_merchant_date,priority:1;index:idx_analytics_merchant_name_date,priority:1"`
	EventID      string `gorm:"size:64;not null;uniqueIndex:uk_analytics_merchant_event,priority:2"`
	Platform     string `gorm:"size:5;not null"`
	DeviceKey    string `gorm:"type:char(64);not null"`
	Name         string `gorm:"size:64;not null;index:idx_analytics_merchant_name_date,priority:2"`
	Page         string `gorm:"size:128;not null"`
	Source       string `gorm:"size:64;not null"`
	ProductID    *uint64
	OccurredAtMS int64  `gorm:"column:occurred_at_ms;not null"`
	EventDate    string `gorm:"type:char(10);not null;index:idx_analytics_merchant_date,priority:2;index:idx_analytics_merchant_name_date,priority:3;index:idx_analytics_date,priority:1"`
	Properties   datatypes.JSON
	CreatedAt    time.Time
}
