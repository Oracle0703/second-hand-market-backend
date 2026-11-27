package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

var (
	analyticsZone  = time.FixedZone("Asia/Shanghai", 8*60*60)
	analyticsID    = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
	analyticsPages = []string{
		"pages/home/index", "pages/category/index", "pages/search/index", "pages/product/list/index",
		"pages/product/detail/index", "pages/favorite/index", "pages/history/index", "pages/login/index",
		"pages/store-guide/index", "pages/me/index",
	}
)

// Keys are UI entry points. Never store free-form labels, search terms or URLs.
var analyticsSources = map[string]struct{ page, name string }{
	"home_carousel":       {"pages/home/index", "product_click"},
	"home_grid":           {"pages/home/index", "product_click"},
	"category_list":       {"pages/category/index", "product_click"},
	"search_results":      {"pages/product/list/index", "product_click"},
	"product_list":        {"pages/product/list/index", "product_click"},
	"favorite_list":       {"pages/favorite/index", "product_click"},
	"history_list":        {"pages/history/index", "product_click"},
	"detail_favorite":     {"pages/product/detail/index", "favorite_click"},
	"detail_unfavorite":   {"pages/product/detail/index", "favorite_click"},
	"favorite_unfavorite": {"pages/favorite/index", "favorite_click"},
	"home_store_phone":    {"pages/home/index", "contact_click"},
	"home_want":           {"pages/home/index", "contact_click"},
	"category_want":       {"pages/category/index", "contact_click"},
	"favorite_want":       {"pages/favorite/index", "contact_click"},
	"detail_want":         {"pages/product/detail/index", "contact_click"},
}

func validAnalyticsPlatform(platform string) bool {
	return platform == "weapp" || platform == "tt"
}

type analyticsEventInput struct {
	EventID    string    `json:"event_id"`
	Name       string    `json:"name"`
	Page       string    `json:"page"`
	Source     string    `json:"source"`
	ProductID  *uint64   `json:"product_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

func (e analyticsEventInput) valid() bool {
	if !analyticsID.MatchString(e.EventID) || !slices.Contains(analyticsPages, e.Page) ||
		e.OccurredAt.IsZero() || (e.ProductID != nil && (*e.ProductID == 0 || *e.ProductID > 1<<53-1)) {
		return false
	}
	name := e.Name
	switch name {
	case "page_view":
		return e.Source == ""
	case "contact_confirm":
		name = "contact_click"
	case "product_click", "favorite_click", "contact_click":
	default:
		return false
	}
	if e.Source == "" {
		return true
	}
	source, ok := analyticsSources[e.Source]
	return ok && source.page == e.Page && source.name == name
}

func (s *Server) handleBuyerAnalyticsEvents(c *gin.Context) {
	if _, err := s.ensureBuyerOrGuest(c); err != nil {
		common.Fail(c, err)
		return
	}
	deviceID := getDeviceID(c)
	if !analyticsID.MatchString(deviceID) {
		common.Fail(c, common.ErrInvalidArgument)
		return
	}
	if err := s.checkRateLimit("buyer:analytics", deviceID, 60, time.Minute); err != nil {
		common.Fail(c, err)
		return
	}
	var req struct {
		Platform string            `json:"platform"`
		Events   []json.RawMessage `json:"events" binding:"required,min=1,max=50"`
	}
	if bindJSON(c, &req) != nil || !validAnalyticsPlatform(req.Platform) {
		common.Fail(c, common.ErrInvalidArgument)
		return
	}
	merchant, err := s.resolveBuyerMerchantScope(c)
	if err != nil {
		common.Fail(c, err)
		return
	}

	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%s:%s", merchant.ID, req.Platform, deviceID)))
	deviceKey := hex.EncodeToString(digest[:])
	now := time.Now()
	earliest, latest := now.Add(-30*24*time.Hour), now.Add(10*time.Minute)
	events := make([]model.MiniappAnalyticsEvent, 0, len(req.Events))
	productIDs := make([]uint64, 0, len(req.Events))
	for _, raw := range req.Events {
		var input analyticsEventInput
		if json.Unmarshal(raw, &input) != nil || !input.valid() ||
			input.OccurredAt.Before(earliest) || input.OccurredAt.After(latest) {
			continue
		}
		events = append(events, model.MiniappAnalyticsEvent{
			MerchantID: merchant.ID, EventID: input.EventID, Platform: req.Platform, DeviceKey: deviceKey,
			Name: input.Name, Page: input.Page, Source: input.Source, ProductID: input.ProductID,
			OccurredAtMS: input.OccurredAt.UnixMilli(), EventDate: input.OccurredAt.In(analyticsZone).Format(time.DateOnly),
		})
		if input.ProductID != nil {
			productIDs = append(productIDs, *input.ProductID)
		}
	}
	db := s.DB.WithContext(c.Request.Context())
	if len(productIDs) > 0 {
		// Offline events can refer to a product removed since the interaction.
		var owned []uint64
		if err := db.Unscoped().Model(&model.Product{}).Where("merchant_id = ? AND id IN ?", merchant.ID, productIDs).
			Pluck("id", &owned).Error; err != nil {
			common.Fail(c, common.ErrInternal)
			return
		}
		for i := range events {
			if id := events[i].ProductID; id != nil && !slices.Contains(owned, *id) {
				events[i].ProductID = nil
			}
		}
	}
	if len(events) > 0 {
		conflict := clause.OnConflict{Columns: []clause.Column{{Name: "merchant_id"}, {Name: "event_id"}}, DoNothing: true}
		if err := db.Clauses(conflict).Create(&events).Error; err != nil {
			common.Fail(c, common.ErrInternal)
			return
		}
	}
	common.Success(c, gin.H{"received": len(req.Events), "discarded": len(req.Events) - len(events)})
}
