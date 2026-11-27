package app

import (
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

type AnalyticsCounts struct {
	PageViews       int64 `json:"page_views"`
	Devices         int64 `json:"devices"`
	ProductClicks   int64 `json:"product_clicks"`
	FavoriteClicks  int64 `json:"favorite_clicks"`
	ContactClicks   int64 `json:"contact_clicks"`
	ContactConfirms int64 `json:"contact_confirms"`
}

type analyticsDay struct {
	Date string `json:"date"`
	AnalyticsCounts
}

type analyticsPage struct {
	Page      string `json:"page"`
	PageViews int64  `json:"page_views"`
	Devices   int64  `json:"devices"`
}

type analyticsAction struct {
	Name      string  `json:"name"`
	Page      string  `json:"page"`
	Source    string  `json:"source"`
	ProductID *uint64 `json:"product_id"`
	Clicks    int64   `json:"clicks"`
}

const analyticsCountSQL = `
	COUNT(CASE WHEN name = 'page_view' THEN 1 END) AS page_views,
	COUNT(DISTINCT CASE WHEN name = 'page_view' THEN device_key END) AS devices,
	COUNT(CASE WHEN name = 'product_click' THEN 1 END) AS product_clicks,
	COUNT(CASE WHEN name = 'favorite_click' THEN 1 END) AS favorite_clicks,
	COUNT(CASE WHEN name = 'contact_click' THEN 1 END) AS contact_clicks,
	COUNT(CASE WHEN name = 'contact_confirm' THEN 1 END) AS contact_confirms`

func analyticsDates(startDate, endDate string) (time.Time, time.Time, bool) {
	start, firstErr := time.ParseInLocation(time.DateOnly, startDate, analyticsZone)
	end, lastErr := time.ParseInLocation(time.DateOnly, endDate, analyticsZone)
	return start, end, firstErr == nil && lastErr == nil && !end.Before(start) && end.Sub(start) < 93*24*time.Hour
}

func (s *Server) handleMerchantAnalytics(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	startDate, endDate := c.Query("start_date"), c.Query("end_date")
	start, end, validDates := analyticsDates(startDate, endDate)
	platform := c.Query("platform")
	if !validDates ||
		(platform != "" && !validAnalyticsPlatform(platform)) {
		common.Fail(c, common.ErrInvalidArgument)
		return
	}

	var summary AnalyticsCounts
	var days []analyticsDay
	pages := []analyticsPage{}
	actions := []analyticsAction{}
	err = s.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&model.MiniappAnalyticsEvent{}).
			Where("merchant_id = ? AND event_date BETWEEN ? AND ?", actor.MerchantID, startDate, endDate)
		if platform != "" {
			query = query.Where("platform = ?", platform)
		}
		// Reuse the scoped filters while keeping each projection and grouping independent.
		query = query.Session(&gorm.Session{})
		if err := query.Select(analyticsCountSQL).Scan(&summary).Error; err != nil {
			return err
		}
		if err := query.Select("event_date AS date, " + analyticsCountSQL).
			Group("event_date").Order("event_date").Scan(&days).Error; err != nil {
			return err
		}
		if err := query.Where("name = ?", "page_view").
			Select("page, COUNT(*) AS page_views, COUNT(DISTINCT device_key) AS devices").
			Group("page").Order("page_views DESC, page ASC").Scan(&pages).Error; err != nil {
			return err
		}
		return query.Where("name IN ?", []string{"product_click", "favorite_click", "contact_click", "contact_confirm"}).
			Select("name, page, source, product_id, COUNT(*) AS clicks").
			Group("name, page, source, product_id").Order("clicks DESC, name, page, source, product_id").
			Limit(100).Scan(&actions).Error
	})
	if err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	daily := make([]analyticsDay, 0, int(end.Sub(start)/(24*time.Hour))+1)
	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		day := analyticsDay{Date: date.Format(time.DateOnly)}
		if len(days) > 0 && days[0].Date == day.Date {
			day, days = days[0], days[1:]
		}
		daily = append(daily, day)
	}
	common.Success(c, gin.H{"summary": summary, "daily": daily, "pages": pages, "actions": actions})
}
