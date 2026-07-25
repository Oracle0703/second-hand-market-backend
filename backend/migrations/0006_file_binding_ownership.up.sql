DROP PROCEDURE IF EXISTS file_binding_ownership_migration;

DELIMITER //
CREATE PROCEDURE file_binding_ownership_migration()
BEGIN
  DECLARE v_count BIGINT DEFAULT 0;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.tables
  WHERE table_schema = DATABASE() AND table_name = 'file_records';
  IF v_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file binding migration: canonical file_records table is required';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'file_records'
    AND column_name = 'owner_merchant_id';
  IF v_count = 0 THEN
    ALTER TABLE file_records ADD COLUMN owner_merchant_id BIGINT UNSIGNED NULL;
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'file_records'
    AND column_name = 'capability_token_hash';
  IF v_count = 0 THEN
    ALTER TABLE file_records ADD COLUMN capability_token_hash CHAR(64) NULL;
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'file_records'
    AND column_name = 'capability_expires_at';
  IF v_count = 0 THEN
    ALTER TABLE file_records ADD COLUMN capability_expires_at DATETIME(3) NULL;
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.statistics
  WHERE table_schema = DATABASE()
    AND table_name = 'file_records'
    AND index_name = 'idx_file_owner_biz_scan';
  IF v_count = 0 THEN
    ALTER TABLE file_records
      ADD INDEX idx_file_owner_biz_scan (owner_merchant_id, biz_type, scan_status);
  ELSEIF v_count <> 3 OR (
    SELECT GROUP_CONCAT(column_name ORDER BY seq_in_index)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'file_records'
      AND index_name = 'idx_file_owner_biz_scan'
  ) <> 'owner_merchant_id,biz_type,scan_status' THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file binding migration: idx_file_owner_biz_scan is drifted';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.statistics
  WHERE table_schema = DATABASE()
    AND table_name = 'file_records'
    AND index_name = 'uk_file_capability_token';
  IF v_count = 0 THEN
    ALTER TABLE file_records
      ADD UNIQUE INDEX uk_file_capability_token (capability_token_hash);
  ELSEIF v_count <> 1 OR (
    SELECT CONCAT(non_unique, ':', column_name)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'file_records'
      AND index_name = 'uk_file_capability_token'
    LIMIT 1
  ) <> '0:capability_token_hash' THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file binding migration: uk_file_capability_token is drifted';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.statistics
  WHERE table_schema = DATABASE()
    AND table_name = 'file_records'
    AND index_name = 'idx_file_capability_expires';
  IF v_count = 0 THEN
    ALTER TABLE file_records
      ADD INDEX idx_file_capability_expires (capability_expires_at);
  ELSEIF v_count <> 1 OR (
    SELECT column_name
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'file_records'
      AND index_name = 'idx_file_capability_expires'
    LIMIT 1
  ) <> 'capability_expires_at' THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file binding migration: idx_file_capability_expires is drifted';
  END IF;

  UPDATE file_records f
  JOIN (
    SELECT refs.file_id, MIN(refs.merchant_id) AS merchant_id
    FROM (
      SELECT pi.file_id, p.merchant_id
      FROM product_images pi
      JOIN products p ON p.id = pi.product_id
      UNION ALL
      SELECT m.license_file_id AS file_id, m.id AS merchant_id
      FROM merchants m
      WHERE m.license_file_id IS NOT NULL
    ) refs
    GROUP BY refs.file_id
  ) bound ON bound.file_id = f.id
  SET f.owner_merchant_id = bound.merchant_id
  WHERE f.owner_merchant_id IS NULL;

  UPDATE file_records f
  JOIN merchant_accounts ma ON ma.id = f.uploader_id
  SET f.owner_merchant_id = ma.merchant_id
  WHERE f.owner_merchant_id IS NULL
    AND f.uploader_type = 'MERCHANT';

  SELECT 'file_binding_ownership_migration_applied' AS migration_gate;
END//
DELIMITER ;

CALL file_binding_ownership_migration();
DROP PROCEDURE file_binding_ownership_migration;
