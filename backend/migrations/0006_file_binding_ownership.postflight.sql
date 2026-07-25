DROP PROCEDURE IF EXISTS file_binding_ownership_postflight;

DELIMITER //
CREATE PROCEDURE file_binding_ownership_postflight()
BEGIN
  DECLARE v_count BIGINT DEFAULT 0;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.tables
  WHERE table_schema = DATABASE() AND table_name = 'files';
  IF v_count <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file binding postflight: legacy files table remains';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'file_records'
    AND column_name IN (
      'owner_merchant_id', 'capability_token_hash', 'capability_expires_at'
    );
  IF v_count <> 3 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file binding postflight: ownership columns are missing';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM (
    SELECT index_name
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'file_records'
      AND index_name = 'idx_file_owner_biz_scan'
    GROUP BY index_name
    HAVING COUNT(*) = 3
      AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'owner_merchant_id,biz_type,scan_status'
  ) expected_owner_index;
  IF v_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file binding postflight: owner/biz/scan index is missing or drifted';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM (
    SELECT index_name
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'file_records'
      AND index_name = 'uk_file_capability_token'
      AND non_unique = 0
    GROUP BY index_name
    HAVING COUNT(*) = 1
      AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'capability_token_hash'
  ) expected_token_index;
  IF v_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file binding postflight: capability token unique index is missing or drifted';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM (
    SELECT index_name
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'file_records'
      AND index_name = 'idx_file_capability_expires'
    GROUP BY index_name
    HAVING COUNT(*) = 1
      AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'capability_expires_at'
  ) expected_expiry_index;
  IF v_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file binding postflight: capability expiry index is missing or drifted';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM product_images pi
  JOIN products p ON p.id = pi.product_id
  LEFT JOIN file_records f ON f.id = pi.file_id
  WHERE f.id IS NULL OR f.owner_merchant_id <> p.merchant_id;
  IF v_count <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file binding postflight: product ownership backfill mismatch';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM merchants m
  LEFT JOIN file_records f ON f.id = m.license_file_id
  WHERE m.license_file_id IS NOT NULL
    AND (f.id IS NULL OR f.owner_merchant_id <> m.id);
  IF v_count <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file binding postflight: license ownership backfill mismatch';
  END IF;
END//
DELIMITER ;

CALL file_binding_ownership_postflight();
DROP PROCEDURE file_binding_ownership_postflight;

SELECT 'file_binding_ownership_postflight_passed' AS migration_gate;
