CREATE TABLE IF NOT EXISTS miniapp_analytics_events (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  merchant_id BIGINT UNSIGNED NOT NULL,
  event_id VARCHAR(64) NOT NULL,
  platform VARCHAR(5) NOT NULL,
  device_key CHAR(64) NOT NULL,
  name VARCHAR(24) NOT NULL,
  page VARCHAR(64) NOT NULL,
  source VARCHAR(32) NOT NULL,
  product_id BIGINT UNSIGNED NULL,
  occurred_at_ms BIGINT NOT NULL,
  event_date CHAR(10) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_analytics_merchant_event (merchant_id, event_id),
  KEY idx_analytics_merchant_date (merchant_id, event_date)
) ENGINE=InnoDB DEFAULT CHARSET=ascii COLLATE=ascii_bin;
