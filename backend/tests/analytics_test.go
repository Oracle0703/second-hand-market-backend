package tests

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"second-hand-market-backend/backend/internal/model"
)

func TestMiniappAnalyticsCollectionAndMerchantReport(t *testing.T) {
	srv := newTestServer(t)
	firstID, username, password := registerMerchant(t, srv, "analytics_a")
	secondID, _, _ := registerMerchant(t, srv, "analytics_b")
	first := merchantNoByID(t, srv, firstID)
	second := merchantNoByID(t, srv, secondID)
	token := str(merchantLogin(t, srv, username, password).Data["access_token"])
	headers := map[string]string{"Authorization": "Bearer " + token}
	firstProduct := createAndOnShelfProduct(t, srv, token)

	zone := time.FixedZone("CST", 8*60*60)
	now := time.Now().In(zone)
	midnight := time.Date(now.Year(), now.Month(), now.Day()-2, 0, 0, 0, 0, zone)
	before := midnight.Add(-time.Second)
	home, detail := "pages/home/index", "pages/product/detail/index"
	event := func(id, name, page, source string, at time.Time) map[string]any {
		return map[string]any{"event_id": id, "name": name, "page": page, "source": source, "occurred_at": at.UTC().Format(time.RFC3339)}
	}
	send := func(store, platform, device string, events ...map[string]any) apiResp {
		return requestJSON(t, srv.Router, http.MethodPost, withMerchantNo("/api/v1/buyer/analytics/events", store),
			map[string]any{"platform": platform, "events": events}, map[string]string{"X-Device-Id": device})
	}
	a := event("e_a", "page_view", home, "", before)
	b := event("e_b", "page_view", detail, "", midnight)
	product := event("e_c", "product_click", home, "home_grid", midnight)
	product["product_id"] = firstProduct
	if r := send(first, "weapp", "device1", a, a); r.Code != 0 {
		t.Fatalf("same-batch duplicate: %+v", r)
	}
	if r := send(first, "weapp", "device1", a, b, product, event("old", "page_view", detail, "", now.AddDate(0, 0, -31))); r.Code != 0 ||
		numToUint64(r.Data["discarded"]) != 1 {
		t.Fatalf("mixed replay, new and expired: %+v", r)
	}
	if r := send(first, "tt", "device1", event("e_d", "page_view", home, "", midnight)); r.Code != 0 {
		t.Fatalf("same device ID on another platform: %+v", r)
	}
	if r := send(first, "weapp", "click_only",
		event("e_e", "favorite_click", detail, "detail_unfavorite", midnight),
		event("e_f", "contact_click", home, "home_store_phone", midnight),
		event("e_g", "contact_confirm", home, "home_store_phone", midnight)); r.Code != 0 {
		t.Fatalf("actions: %+v", r)
	}
	if r := send(second, "weapp", "device1", a, product); r.Code != 0 {
		t.Fatalf("same event ID in another merchant: %+v", r)
	}
	var foreign model.MiniappAnalyticsEvent
	if err := srv.DB.Where("merchant_id = ? AND event_id = ?", secondID, "e_c").Take(&foreign).Error; err != nil || foreign.ProductID != nil {
		t.Fatalf("foreign product attribution = %v, err = %v", foreign.ProductID, err)
	}

	start := before.Format(time.DateOnly)
	end := midnight.AddDate(0, 0, 1).Format(time.DateOnly)
	url := fmt.Sprintf("/api/v1/merchant/analytics?start_date=%s&end_date=%s", start, end)
	report := requestJSON(t, srv.Router, http.MethodGet, url+"&merchant_no="+second, nil, headers)
	if report.Code != 0 {
		t.Fatalf("report: %+v", report)
	}
	summary := report.Data["summary"].(map[string]any)
	want := map[string]any{"page_views": 3., "devices": 2., "product_clicks": 1., "favorite_clicks": 1., "contact_clicks": 1., "contact_confirms": 1.}
	if !reflect.DeepEqual(summary, want) {
		t.Fatalf("summary = %v, want %v", summary, want)
	}
	days := report.Data["daily"].([]any)
	if len(days) != 3 {
		t.Fatalf("daily length = %d", len(days))
	}
	for i, expected := range []struct {
		date           string
		views, devices float64
	}{{start, 1, 1}, {midnight.Format(time.DateOnly), 2, 2}, {end, 0, 0}} {
		day := days[i].(map[string]any)
		if day["date"] != expected.date || day["page_views"] != expected.views || day["devices"] != expected.devices {
			t.Fatalf("daily[%d] = %+v", i, day)
		}
	}
	pages := report.Data["pages"].([]any)
	if len(pages) != 2 || !reflect.DeepEqual(pages[0], map[string]any{"page": home, "page_views": 2., "devices": 2.}) {
		t.Fatalf("pages = %+v", pages)
	}
	actions := report.Data["actions"].([]any)
	if len(actions) != 4 {
		t.Fatalf("actions = %+v", actions)
	}
	last := actions[3].(map[string]any)
	if last["name"] != "product_click" || last["source"] != "home_grid" || last["product_id"] != float64(firstProduct) || last["clicks"] != 1. {
		t.Fatalf("product action = %+v", last)
	}
	filtered := requestJSON(t, srv.Router, http.MethodGet, url+"&platform=tt", nil, headers)
	if filtered.Code != 0 || filtered.Data["summary"].(map[string]any)["page_views"] != 1. || len(filtered.Data["actions"].([]any)) != 0 {
		t.Fatalf("platform filter = %+v", filtered)
	}
	if r := requestJSON(t, srv.Router, http.MethodGet, url, nil, nil); r.HTTPStatus != http.StatusUnauthorized {
		t.Fatalf("guest report = %+v", r)
	}
	if r := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/analytics?start_date=2026-01-01&end_date=2026-04-04", nil, headers); r.Code != 10001 {
		t.Fatalf("overlong date range = %+v", r)
	}
}

func TestMiniappAnalyticsIsolatesInvalidEventsAndRejectsBadEnvelopes(t *testing.T) {
	srv := newTestServer(t)
	merchantID, _, _ := registerMerchant(t, srv, "analytics_invalid")
	path := withMerchantNo("/api/v1/buyer/analytics/events", merchantNoByID(t, srv, merchantID))
	stamp := time.Now().UTC().Format(time.RFC3339)
	valid := map[string]any{"event_id": "e_ok", "name": "page_view", "page": "pages/home/index", "occurred_at": stamp}
	invalid := []any{
		map[string]any{"event_id": "e_bad", "name": "contact_click", "page": "pages/home/index", "source": "arbitrary label", "occurred_at": stamp},
		map[string]any{"event_id": "e_bad2", "name": "page_view", "page": "pages/home/index", "occurred_at": "invalid timestamp"},
		map[string]any{"event_id": "e_bad3", "name": "page_view", "page": "pages/home/index", "occurred_at": stamp, "product_id": "unknown"},
		"broken event",
	}
	for _, validNeighbor := range []bool{false, true} {
		batch := invalid
		if validNeighbor {
			batch = append([]any{valid}, invalid...)
		}
		r := requestJSON(t, srv.Router, http.MethodPost, path,
			map[string]any{"platform": "weapp", "events": batch}, map[string]string{"X-Device-Id": "dev"})
		if r.Code != 0 || r.Data["received"] != float64(len(batch)) || r.Data["discarded"] != float64(len(invalid)) {
			t.Fatalf("per-event acknowledgement = %+v", r)
		}
	}
	for _, tc := range []struct {
		platform, device string
		events           []map[string]any
	}{
		{"web", "dev", []map[string]any{valid}},
		{"tt", "", []map[string]any{valid}},
		{"tt", "dev", []map[string]any{}},
		{"weapp", "dev", make([]map[string]any, 51)},
	} {
		r := requestJSON(t, srv.Router, http.MethodPost, path, map[string]any{"platform": tc.platform, "events": tc.events}, map[string]string{"X-Device-Id": tc.device})
		if r.Code != 10001 {
			t.Fatalf("invalid batch accepted: %+v", r)
		}
	}
	var count int64
	if err := srv.DB.Model(&model.MiniappAnalyticsEvent{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("events = %d err = %v", count, err)
	}
}
