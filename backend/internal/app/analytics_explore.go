package app

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

type propertyFilter struct {
	Key   string `json:"key"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

func (f propertyFilter) valid() bool {
	if !validAnalyticsPropertyKey(f.Key) || !validAnalyticsScalar(f.Value) {
		return false
	}
	switch f.Op {
	case "eq", "ne":
		return true
	case "gte", "lte":
		_, ok := f.Value.(float64)
		return ok
	case "contains":
		_, ok := f.Value.(string)
		return ok
	default:
		return false
	}
}

func positiveQueryID(value string) (uint64, bool) {
	id, err := strconv.ParseUint(value, 10, 64)
	return id, err == nil && id > 0 && id <= 1<<53-1
}

// Every projection, including dimension discovery, starts with this tenant/date scope.
func scopeAnalytics(c *gin.Context, tx *gorm.DB) (*gorm.DB, error) {
	actor, err := actorFromContext(c)
	if err != nil {
		return nil, err
	}
	start, end := c.Query("start_date"), c.Query("end_date")
	if _, _, valid := analyticsDates(start, end); !valid {
		return nil, common.ErrInvalidArgument
	}
	q := tx.Table("miniapp_analytics_events AS e").Where("e.event_date BETWEEN ? AND ?", start, end)
	if actor.UserType == model.UserTypeMerchant {
		if c.Request.URL.Query().Has("merchant_id") {
			return nil, common.ErrForbidden
		}
		q = q.Where("e.merchant_id = ?", actor.MerchantID)
	} else if actor.UserType == model.UserTypeAdmin {
		if v, present := c.GetQuery("merchant_id"); present {
			id, ok := positiveQueryID(v)
			if !ok {
				return nil, common.ErrInvalidArgument
			}
			q = q.Where("e.merchant_id = ?", id)
		}
	} else {
		return nil, common.ErrForbidden
	}
	if v := c.Query("platform"); v != "" {
		if !validAnalyticsPlatform(v) {
			return nil, common.ErrInvalidArgument
		}
		q = q.Where("e.platform = ?", v)
	}
	if v := c.Query("name"); v != "" {
		if !validAnalyticsKey(v, 64) {
			return nil, common.ErrInvalidArgument
		}
		q = q.Where("e.name = ?", v)
	}
	return q, nil
}

func applyExploreFilters(c *gin.Context, q *gorm.DB) (*gorm.DB, error) {
	if v := c.Query("page"); v != "" {
		if len(v) > 128 || !analyticsRoute.MatchString(v) {
			return nil, common.ErrInvalidArgument
		}
		q = q.Where(analyticsPageColumn(q)+" = ?", v)
	}
	if v := c.Query("source"); v != "" {
		if !validAnalyticsKey(v, 64) {
			return nil, common.ErrInvalidArgument
		}
		q = q.Where("e.source = ?", v)
	}
	if v := c.Query("product_id"); v != "" {
		id, ok := positiveQueryID(v)
		if !ok {
			return nil, common.ErrInvalidArgument
		}
		q = q.Where("e.product_id = ?", id)
	}
	if raw := c.Query("filters"); raw != "" {
		var filters []propertyFilter
		if len(raw) > 12000 || json.Unmarshal([]byte(raw), &filters) != nil || filters == nil || len(filters) > 8 {
			return nil, common.ErrInvalidArgument
		}
		for _, f := range filters {
			if !f.valid() {
				return nil, common.ErrInvalidArgument
			}
			q = filterAnalyticsProperty(q, f)
		}
	}
	return q.Session(&gorm.Session{}), nil
}

// Normalize both engines' JSON type vocabularies. A missing key remains SQL NULL.
func propertyKindSQL(mysql bool) string {
	if mysql {
		return "CASE JSON_TYPE(JSON_EXTRACT(e.properties, ?)) WHEN 'STRING' THEN 'string' WHEN 'BOOLEAN' THEN 'boolean' WHEN 'INTEGER' THEN 'number' WHEN 'DOUBLE' THEN 'number' WHEN 'DECIMAL' THEN 'number' END"
	}
	return "CASE json_type(e.properties, ?) WHEN 'text' THEN 'string' WHEN 'true' THEN 'boolean' WHEN 'false' THEN 'boolean' WHEN 'integer' THEN 'number' WHEN 'real' THEN 'number' END"
}

func filterAnalyticsProperty(q *gorm.DB, f propertyFilter) *gorm.DB {
	mysql := q.Dialector.Name() == "mysql"
	path := "$." + f.Key
	extract := "json_extract(e.properties, ?)"
	var kind string
	value := f.Value
	switch v := value.(type) {
	case string:
		kind = "string"
		if mysql {
			extract = "CAST(JSON_UNQUOTE(JSON_EXTRACT(e.properties, ?)) AS BINARY)"
		}
	case float64:
		kind = "number"
		if mysql {
			extract = "CAST(JSON_UNQUOTE(JSON_EXTRACT(e.properties, ?)) AS DOUBLE)"
		}
	case bool:
		kind = "boolean"
		if mysql {
			extract = "JSON_UNQUOTE(JSON_EXTRACT(e.properties, ?))"
			value = strconv.FormatBool(v)
		}
	}
	q = q.Where(propertyKindSQL(mysql)+" = ?", path, kind)
	if f.Op == "contains" {
		return q.Where("INSTR("+extract+", ?) > 0", path, value)
	}
	op := map[string]string{"eq": "=", "ne": "<>", "gte": ">=", "lte": "<="}[f.Op]
	return q.Where(extract+" "+op+" ?", path, value)
}

type exploreGroup struct {
	Value       *string `json:"value" gorm:"column:group_value"`
	ValueType   *string `json:"value_type" gorm:"column:group_type"`
	Count       int64   `json:"count"`
	Devices     int64   `json:"devices"`
	GroupsTotal int64   `json:"-"`
}

// AutoMigrate MySQL databases may use a case-insensitive default collation.
func analyticsPageColumn(q *gorm.DB) string {
	if q.Dialector.Name() == "mysql" {
		return "CAST(e.page AS BINARY)"
	}
	return "e.page"
}

func groupProjection(q *gorm.DB, key string, admin bool) (*gorm.DB, error) {
	columns := map[string]string{
		"name": "e.name", "date": "e.event_date", "platform": "e.platform",
		"page": "NULLIF(" + analyticsPageColumn(q) + ", '')", "source": "NULLIF(e.source, '')",
		"product_id": "e.product_id", "merchant_id": "e.merchant_id",
	}
	if key == "merchant_id" && !admin {
		return nil, common.ErrForbidden
	}
	if col, ok := columns[key]; ok {
		kind := "string"
		valueSQL := col
		if key == "merchant_id" || key == "product_id" {
			kind = "number"
			valueSQL = "CAST(" + col + " AS CHAR)"
		}
		return q.Select("e.device_key, "+valueSQL+" AS group_value, CASE WHEN "+col+" IS NOT NULL THEN ? END AS group_type", kind), nil
	}
	prop, ok := strings.CutPrefix(key, "prop.")
	if !ok || !validAnalyticsPropertyKey(prop) {
		return nil, common.ErrInvalidArgument
	}
	path := "$." + prop
	if q.Dialector.Name() == "mysql" {
		return q.Select("e.device_key, CAST(JSON_UNQUOTE(JSON_EXTRACT(e.properties, ?)) AS BINARY) AS group_value, "+propertyKindSQL(true)+" AS group_type", path, path), nil
	}
	// The -> operator preserves the JSON spelling of numbers, unlike SQLite's
	// default 15-digit conversion to TEXT. For strings use the decoded value.
	return q.Select("e.device_key, CASE json_type(e.properties, ?) WHEN 'text' THEN json_extract(e.properties, ?) ELSE e.properties -> ? END AS group_value, "+propertyKindSQL(false)+" AS group_type", path, path, path, path), nil
}

type exploreEvent struct {
	EventID      string         `json:"event_id"`
	Name         string         `json:"name"`
	Page         string         `json:"page"`
	Source       string         `json:"source"`
	ProductID    *uint64        `json:"product_id"`
	Platform     string         `json:"platform"`
	MerchantID   uint64         `json:"merchant_id"`
	Properties   datatypes.JSON `json:"properties"`
	OccurredAtMS int64          `json:"-" gorm:"column:occurred_at_ms"`
	OccurredAt   string         `json:"occurred_at" gorm:"-"`
}

func (s *Server) handleAnalyticsExplore(c *gin.Context) {
	page, pageErr := strconv.ParseInt(c.DefaultQuery("page_no", "1"), 10, 32)
	size, sizeErr := strconv.ParseInt(c.DefaultQuery("page_size", "20"), 10, 32)
	if pageErr != nil || sizeErr != nil || page < 1 || size < 1 || size > 100 {
		common.Fail(c, common.ErrInvalidArgument)
		return
	}
	groupBy := c.DefaultQuery("group_by", "name")
	var total struct{ Total, Devices int64 }
	groups := []exploreGroup{}
	events := []exploreEvent{}
	err := s.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		q, err := scopeAnalytics(c, tx)
		if err != nil {
			return err
		}
		q, err = applyExploreFilters(c, q)
		if err != nil {
			return err
		}
		actor, _ := common.GetActor(c)
		projection, err := groupProjection(q, groupBy, actor.UserType == model.UserTypeAdmin)
		if err != nil {
			return err
		}
		if err := q.Select("COUNT(*) AS total, COUNT(DISTINCT e.device_key) AS devices").Scan(&total).Error; err != nil {
			return err
		}
		if err := tx.Table("(?) AS selected", projection).
			Select("group_value, group_type, COUNT(*) AS count, COUNT(DISTINCT device_key) AS devices, COUNT(*) OVER () AS groups_total").
			Group("group_type, group_value").Order("count DESC, group_type ASC, group_value ASC").Limit(100).Scan(&groups).Error; err != nil {
			return err
		}
		return q.Select("e.event_id, e.name, e.page, e.source, e.product_id, e.platform, e.merchant_id, e.properties, e.occurred_at_ms").
			Order("e.occurred_at_ms DESC, e.id DESC").Offset(int((page - 1) * size)).Limit(int(size)).Scan(&events).Error
	})
	if err != nil {
		common.Fail(c, err)
		return
	}
	var groupCount int64
	if len(groups) > 0 {
		groupCount = groups[0].GroupsTotal
	}
	for i := range groups {
		g := &groups[i]
		if strings.HasPrefix(groupBy, "prop.") && g.ValueType != nil && *g.ValueType == "number" && g.Value != nil {
			if n, err := strconv.ParseFloat(*g.Value, 64); err == nil {
				b, _ := json.Marshal(n)
				v := string(b)
				g.Value = &v
			}
		}
	}
	for i := range events {
		ev := &events[i]
		ev.OccurredAt = time.UnixMilli(ev.OccurredAtMS).UTC().Format(time.RFC3339Nano)
		if len(ev.Properties) == 0 || string(ev.Properties) == "null" {
			ev.Properties = datatypes.JSON("{}")
		}
	}
	common.Success(c, gin.H{"total": total.Total, "devices": total.Devices, "groups": groups, "groups_total": groupCount, "events": events, "page_no": page, "page_size": size})
}

func (s *Server) handleAnalyticsDimensions(c *gin.Context) {
	result := map[string][]string{"names": {}, "pages": {}, "sources": {}, "property_keys": {}}
	err := s.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		q, err := scopeAnalytics(c, tx)
		if err != nil {
			return err
		}
		q = q.Session(&gorm.Session{})
		for _, field := range []struct{ key, col string }{{"names", "e.name"}, {"pages", analyticsPageColumn(q)}, {"sources", "e.source"}} {
			values := []string{}
			if err := q.Where(field.col + " <> ''").Select("DISTINCT " + field.col + " AS candidate").Order("candidate").Limit(200).Scan(&values).Error; err != nil {
				return err
			}
			result[field.key] = values
		}
		key := "j.key"
		join := "JOIN json_each(e.properties) AS j ON TRUE"
		if tx.Dialector.Name() == "mysql" {
			key = "j.prop_key"
			join = "JOIN JSON_TABLE(COALESCE(JSON_KEYS(e.properties), JSON_ARRAY()), '$[*]' COLUMNS(prop_key VARCHAR(48) PATH '$')) AS j ON TRUE"
		}
		keys := []string{}
		if err := q.Joins(join).Distinct(key).Order(key).Limit(200).Pluck(key, &keys).Error; err != nil {
			return err
		}
		result["property_keys"] = keys
		return nil
	})
	if err != nil {
		common.Fail(c, err)
		return
	}
	common.Success(c, result)
}
