DROP PROCEDURE IF EXISTS migrate_analytics_properties;
DELIMITER //
CREATE PROCEDURE migrate_analytics_properties()
BEGIN
  IF (SELECT COUNT(*) FROM information_schema.columns
      WHERE table_schema = DATABASE() AND table_name = 'miniapp_analytics_events'
      AND ((column_name = 'name' AND character_maximum_length = 64)
        OR (column_name = 'page' AND character_maximum_length = 128)
        OR (column_name = 'source' AND character_maximum_length = 64))) <> 3 THEN
    ALTER TABLE miniapp_analytics_events
      MODIFY name VARCHAR(64) NOT NULL, MODIFY page VARCHAR(128) NOT NULL,
      MODIFY source VARCHAR(64) NOT NULL;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE()
      AND table_name = 'miniapp_analytics_events' AND column_name = 'properties') THEN
    ALTER TABLE miniapp_analytics_events ADD COLUMN properties JSON NULL;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.statistics WHERE table_schema = DATABASE()
      AND table_name = 'miniapp_analytics_events' AND index_name = 'idx_analytics_merchant_name_date') THEN
    CREATE INDEX idx_analytics_merchant_name_date ON miniapp_analytics_events (merchant_id, name, event_date);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.statistics WHERE table_schema = DATABASE()
      AND table_name = 'miniapp_analytics_events' AND index_name = 'idx_analytics_date') THEN
    CREATE INDEX idx_analytics_date ON miniapp_analytics_events (event_date);
  END IF;
END//
DELIMITER ;
CALL migrate_analytics_properties();
DROP PROCEDURE migrate_analytics_properties;
