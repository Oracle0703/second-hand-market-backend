package model

import "testing"

func TestSQLiteAnalyticsUpgradeKeepsLegacyEvents(t *testing.T) {
	db := newModelSchemaTestDB(t)
	err := db.Exec(`CREATE TABLE miniapp_analytics_events (
    id integer primary key autoincrement, merchant_id integer not null, event_id text not null,
    platform text not null, device_key char(64) not null, name text not null, page text not null,
    source text not null, product_id integer, occurred_at_ms integer not null,
    event_date char(10) not null, created_at datetime)`).Error
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO miniapp_analytics_events
        (merchant_id,event_id,platform,device_key,name,page,source,occurred_at_ms,event_date)
        VALUES(1,'before','tt','device','page_view','pages/home/index','',0,'1970-01-01')`).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := db.AutoMigrate(&MiniappAnalyticsEvent{}); err != nil {
			t.Fatal(err)
		}
	}
	var n int64
	if err := db.Model(&MiniappAnalyticsEvent{}).Where("event_id=? AND properties IS NULL", "before").Count(&n).Error; err != nil || n != 1 {
		t.Fatalf("legacy rows %d, err %v", n, err)
	}
	for _, index := range []string{"uk_analytics_merchant_event", "idx_analytics_merchant_name_date", "idx_analytics_date"} {
		if !db.Migrator().HasIndex(&MiniappAnalyticsEvent{}, index) {
			t.Fatalf("missing index %s", index)
		}
	}
}
