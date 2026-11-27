package model

import "time"

type MiniappAnalyticsEvent struct {
	ID           uint64 `gorm:"primaryKey"`
	MerchantID   uint64 `gorm:"not null;uniqueIndex:uk_analytics_merchant_event,priority:1;index:idx_analytics_merchant_date,priority:1"`
	EventID      string `gorm:"size:64;not null;uniqueIndex:uk_analytics_merchant_event,priority:2"`
	Platform     string `gorm:"size:5;not null"`
	DeviceKey    string `gorm:"type:char(64);not null"`
	Name         string `gorm:"size:24;not null"`
	Page         string `gorm:"size:64;not null"`
	Source       string `gorm:"size:32;not null"`
	ProductID    *uint64
	OccurredAtMS int64  `gorm:"column:occurred_at_ms;not null"`
	EventDate    string `gorm:"type:char(10);not null;index:idx_analytics_merchant_date,priority:2"`
	CreatedAt    time.Time
}
