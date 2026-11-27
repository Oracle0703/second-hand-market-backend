package tests

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"reflect"
	"testing"
	"time"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/model"
)

func TestCustomAnalyticsExplore(t *testing.T) {
	checkAnalyticsExplorer(t, newTestServer(t))
}

func checkAnalyticsExplorer(t *testing.T, s *app.Server) {
	t.Helper()
	one, u, p := registerMerchant(t, s, "events_one")
	two, _, _ := registerMerchant(t, s, "events_two")
	mt := str(merchantLogin(t, s, u, p).Data["access_token"])
	at := adminAccessToken(t, s)
	stamp := time.Now().UTC()
	day := stamp.In(time.FixedZone("CST", 8*3600)).Format(time.DateOnly)
	write := func(m uint64, events []map[string]any) apiResp {
		return requestJSON(t, s.Router, http.MethodPost, withMerchantNo("/api/v1/buyer/analytics/events", merchantNoByID(t, s, m)),
			map[string]any{"platform": "tt", "events": events}, map[string]string{"X-Device-Id": "generic_install"})
	}
	event := func(id int, props map[string]any) map[string]any {
		return map[string]any{"event_id": fmt.Sprintf("buy_%d", id), "name": "order_created", "occurred_at": stamp.Add(time.Duration(id) * time.Millisecond).Format(time.RFC3339Nano), "properties": props}
	}
	batch := []map[string]any{
		event(1, map[string]any{"variant": 1, "amount": 9.25, "label": "蓝色 100%_", "repeatable": true}),
		event(2, map[string]any{"variant": "1", "amount": 10.5, "label": "蓝色杯", "repeatable": false}),
		event(3, map[string]any{"variant": true, "amount": "10.5", "label": "test"}),
		event(4, map[string]any{"variant": "true", "amount": 2, "label": "特殊"}),
		event(5, nil),
		event(6, map[string]any{"access_token": "private"}),
		event(7, map[string]any{"nested": []int{1}}),
	}
	batch[0]["page"] = "customShop/order/detail"
	batch[0]["source"] = "new_touchpoint"
	batch[1]["page"] = "customshop/order/detail"
	for i := 0; i < 2; i++ {
		res := write(one, batch)
		if res.Code != 0 || res.Data["received"] != 7. || res.Data["discarded"] != 2. {
			t.Fatalf("batch: %+v", res)
		}
		rejected := res.Data["rejected"].([]any)
		if len(rejected) != 2 || !reflect.DeepEqual(rejected[0], map[string]any{"index": 5., "reason": "restricted_property"}) {
			t.Fatalf("rejected: %+v", rejected)
		}
	}
	if r := write(two, []map[string]any{event(1, map[string]any{"other_key": true})}); r.Code != 0 || r.Data["discarded"] != 0. {
		t.Fatalf("second scope: %+v", r)
	}
	legacy := model.MiniappAnalyticsEvent{MerchantID: one, DeviceKey: "legacy_device", EventID: "old_page", Platform: "tt", Name: "page_view", Page: "pages/home/index", OccurredAtMS: stamp.UnixMilli(), EventDate: day}
	if err := s.DB.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	read := func(admin bool, endpoint string, args url.Values) apiResp {
		scope, token := "merchant", mt
		if admin {
			scope, token = "admin", at
		}
		v := url.Values{"start_date": {day}, "end_date": {day}, "platform": {"tt"}}
		for k, val := range args {
			v[k] = val
		}
		return requestJSON(t, s.Router, http.MethodGet, "/api/v1/"+scope+"/analytics"+endpoint+"?"+v.Encode(), nil, map[string]string{"Authorization": "Bearer " + token})
	}
	args := url.Values{"name": {"order_created"}, "group_by": {"prop.variant"}, "page_size": {"2"}, "page_no": {"2"}}
	res := read(false, "/explore", args)
	if res.Code != 0 || res.Data["total"] != 5. || res.Data["devices"] != 1. || res.Data["groups_total"] != 5. {
		t.Fatalf("explore: %+v", res)
	}
	sets := map[string]bool{}
	for _, item := range res.Data["groups"].([]any) {
		v := item.(map[string]any)
		sets[fmt.Sprintf("%v=%v", v["value_type"], v["value"])] = true
	}
	for _, g := range []string{"number=1", "string=1", "boolean=true", "string=true", "<nil>=<nil>"} {
		if !sets[g] {
			t.Fatalf("missing typed group %q: %+v", g, sets)
		}
	}
	rows := res.Data["events"].([]any)
	if len(rows) != 2 || rows[0].(map[string]any)["event_id"] != "buy_3" || rows[1].(map[string]any)["event_id"] != "buy_2" {
		t.Fatalf("page 2: %+v", rows)
	}
	checkFilters := func(want float64, filters ...map[string]any) {
		t.Helper()
		encoded, _ := json.Marshal(filters)
		r := read(false, "/explore", url.Values{"name": {"order_created"}, "filters": {string(encoded)}})
		if r.Code != 0 || r.Data["total"] != want {
			t.Fatalf("filters %+v: %+v", filters, r)
		}
	}
	filter := func(k, op string, v any) map[string]any { return map[string]any{"key": k, "op": op, "value": v} }
	checkFilters(1, filter("amount", "gte", 9), filter("amount", "lte", 10), filter("label", "contains", "蓝色"))
	checkFilters(1, filter("label", "contains", "%_"))
	checkFilters(1, filter("repeatable", "eq", true))
	checkFilters(1, filter("repeatable", "ne", true))
	checkFilters(2, filter("amount", "ne", 10.5))
	checkFilters(1, filter("amount", "eq", "10.5"))
	checkFilters(1, filter("label", "eq", "蓝色杯"))
	checkFilters(0, filter("label", "eq", "蓝色杯 "))
	legacyRes := read(false, "/explore", url.Values{"name": {"page_view"}})
	if legacyRes.Code != 0 || !reflect.DeepEqual(legacyRes.Data["events"].([]any)[0].(map[string]any)["properties"], map[string]any{}) {
		t.Fatalf("legacy row: %+v", legacyRes)
	}
	oldReport := read(false, "", nil)
	if oldReport.Code != 0 || len(oldReport.Data["actions"].([]any)) != 0 {
		t.Fatalf("custom event leaked into old report: %+v", oldReport)
	}
	dims := read(false, "/dimensions", url.Values{"name": {"order_created"}})
	if dims.Code != 0 || !reflect.DeepEqual(dims.Data["property_keys"], []any{"amount", "label", "repeatable", "variant"}) ||
		!reflect.DeepEqual(dims.Data["pages"], []any{"customShop/order/detail", "customshop/order/detail"}) || !reflect.DeepEqual(dims.Data["sources"], []any{"new_touchpoint"}) {
		t.Fatalf("dimensions: %+v", dims)
	}
	adminRes := read(true, "/explore", url.Values{"name": {"order_created"}, "group_by": {"merchant_id"}})
	if adminRes.Code != 0 || adminRes.Data["total"] != 6. || adminRes.Data["devices"] != 2. || adminRes.Data["groups_total"] != 2. {
		t.Fatalf("admin: %+v", adminRes)
	}
	adminDims := read(true, "/dimensions", url.Values{"merchant_id": {fmt.Sprint(two)}})
	if adminDims.Code != 0 || !reflect.DeepEqual(adminDims.Data["property_keys"], []any{"other_key"}) {
		t.Fatalf("admin dimensions: %+v", adminDims)
	}
	for _, tc := range []struct {
		path   string
		params url.Values
		code   int
	}{
		{"/explore", url.Values{"merchant_id": {fmt.Sprint(two)}}, 403}, {"/dimensions", url.Values{"merchant_id": {fmt.Sprint(two)}}, 403},
		{"/explore", url.Values{"group_by": {"merchant_id"}}, 403}, {"/explore", url.Values{"group_by": {"prop.access_token"}}, 400},
		{"/explore", url.Values{"page_size": {"101"}}, 400}, {"/explore", url.Values{"filters": {`[{"key":"amount","op":"gte","value":"2"}]`}}, 400},
	} {
		if r := read(false, tc.path, tc.params); r.HTTPStatus != tc.code {
			t.Fatalf("invalid %s %+v: %+v", tc.path, tc.params, r)
		}
	}

	pageGroups := read(false, "/explore", url.Values{"name": {"order_created"}, "group_by": {"page"}})
	pageFilter := read(false, "/explore", url.Values{"name": {"order_created"}, "page": {"customShop/order/detail"}})
	if pageGroups.Code != 0 || pageGroups.Data["groups_total"] != 3. || pageFilter.Code != 0 || pageFilter.Data["total"] != 1. {
		t.Fatalf("case-sensitive routes: groups=%+v filter=%+v", pageGroups, pageFilter)
	}
	// A truncated group list must still report the exact cardinality.
	many := make([]map[string]any, 0, 105)
	for i := 0; i < 105; i++ {
		value := float64(i)
		if i == 104 {
			value = math.Copysign(0, -1)
		}
		e := event(100+i, map[string]any{"bucket": value})
		e["name"] = "catalog_sample"
		many = append(many, e)
	}
	for len(many) > 0 {
		n := min(len(many), 50)
		if r := write(one, many[:n]); r.Code != 0 || r.Data["discarded"] != 0. {
			t.Fatalf("large sample: %+v", r)
		}
		many = many[n:]
	}
	capped := read(false, "/explore", url.Values{"name": {"catalog_sample"}, "group_by": {"prop.bucket"}})
	if capped.Code != 0 || capped.Data["total"] != 105. || capped.Data["groups_total"] != 104. {
		t.Fatalf("full cardinality: %+v", capped)
	}
	cappedGroups := capped.Data["groups"].([]any)
	if len(cappedGroups) != 100 || cappedGroups[0].(map[string]any)["value"] != "0" || cappedGroups[0].(map[string]any)["count"] != 2. {
		t.Fatalf("truncated groups: %+v", cappedGroups)
	}
}
