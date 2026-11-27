package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm/clause"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

var (
	analyticsZone       = time.FixedZone("Asia/Shanghai", 8*60*60)
	analyticsID         = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
	analyticsKey        = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	analyticsRoute      = regexp.MustCompile(`^[A-Za-z0-9_-]+(/[A-Za-z0-9_-]+)+$`)
	privateAnalyticsKey = regexp.MustCompile(`(^|_)(token|password|passwd|secret|credential|authorization|cookie|phone|mobile|email|openid|unionid|address|idcard|session|id_card|open_id|union_id|access_key|api_key)($|_)`)
)

func validAnalyticsKey(value string, limit int) bool {
	return len(value) <= limit && analyticsKey.MatchString(value)
}

func validAnalyticsPropertyKey(key string) bool {
	return validAnalyticsKey(key, 48) && !privateAnalyticsKey.MatchString(key)
}

func validAnalyticsScalar(value any) bool {
	switch v := value.(type) {
	case string:
		return utf8.RuneCountInString(v) <= 128 && strings.IndexFunc(v, unicode.IsControl) < 0
	case float64:
		return math.Abs(v) <= 1<<53-1
	case bool:
		return true
	default:
		return false
	}
}

func validAnalyticsPlatform(platform string) bool {
	return platform == "weapp" || platform == "tt"
}

type analyticsEventInput struct {
	EventID    string         `json:"event_id"`
	Name       string         `json:"name"`
	Page       string         `json:"page"`
	Source     string         `json:"source"`
	ProductID  *uint64        `json:"product_id"`
	Properties map[string]any `json:"properties"`
	OccurredAt time.Time      `json:"occurred_at"`
}

func (e analyticsEventInput) rejectionReason() string {
	switch {
	case !analyticsID.MatchString(e.EventID):
		return "invalid_event_id"
	case !validAnalyticsKey(e.Name, 64):
		return "invalid_name"
	case (e.Page == "" && e.Name == "page_view") || (e.Page != "" && (len(e.Page) > 128 || !analyticsRoute.MatchString(e.Page))):
		return "invalid_page"
	case e.Source != "" && !validAnalyticsKey(e.Source, 64):
		return "invalid_source"
	case e.OccurredAt.IsZero():
		return "invalid_time"
	case e.ProductID != nil && (*e.ProductID == 0 || *e.ProductID > 1<<53-1):
		return "invalid_product_id"
	case len(e.Properties) > 16:
		return "too_many_properties"
	}
	for key, value := range e.Properties {
		if !validAnalyticsKey(key, 48) {
			return "invalid_property_key"
		}
		if privateAnalyticsKey.MatchString(key) {
			return "restricted_property"
		}
		if !validAnalyticsScalar(value) {
			return "invalid_property_value"
		}
	}
	return ""
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
	type rejection struct {
		Index  int    `json:"index"`
		Reason string `json:"reason"`
	}
	rejected := []rejection{}
	for i, raw := range req.Events {
		var input analyticsEventInput
		reason := "malformed_event"
		if json.Unmarshal(raw, &input) == nil {
			reason = input.rejectionReason()
			if reason == "" && (input.OccurredAt.Before(earliest) || input.OccurredAt.After(latest)) {
				reason = "time_out_of_range"
			}
		}
		if reason != "" {
			rejected = append(rejected, rejection{i, reason})
			continue
		}
		var properties datatypes.JSON
		if len(input.Properties) > 0 {
			// JSON permits negative zero. Store a single representation for zero
			// so SQLite and MySQL group all numerically equal zeros together.
			for key, value := range input.Properties {
				if n, ok := value.(float64); ok && n == 0 {
					input.Properties[key] = float64(0)
				}
			}
			encoded, err := json.Marshal(input.Properties)
			if err != nil {
				rejected = append(rejected, rejection{i, "invalid_properties"})
				continue
			}
			properties = encoded
		}
		events = append(events, model.MiniappAnalyticsEvent{
			MerchantID: merchant.ID, EventID: input.EventID, Platform: req.Platform, DeviceKey: deviceKey, Properties: properties,
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
	common.Success(c, gin.H{"received": len(req.Events), "discarded": len(rejected), "rejected": rejected})
}
