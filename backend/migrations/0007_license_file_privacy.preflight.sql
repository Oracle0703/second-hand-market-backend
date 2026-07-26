DROP PROCEDURE IF EXISTS license_file_privacy_preflight;

DELIMITER //
CREATE PROCEDURE license_file_privacy_preflight()
BEGIN
  DECLARE v_count BIGINT DEFAULT 0;
  DECLARE v_bad_license BIGINT DEFAULT 0;
  DECLARE v_bad_bound_license BIGINT DEFAULT 0;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.tables
  WHERE table_schema = DATABASE() AND table_name = 'file_records';
  IF v_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'license privacy preflight: canonical file_records table is required';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.tables
  WHERE table_schema = DATABASE() AND table_name = 'files';
  IF v_count <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'license privacy preflight: legacy files table must not exist';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'file_records'
    AND column_name = 'owner_merchant_id'
    AND column_type = 'bigint unsigned'
    AND is_nullable = 'YES';
  IF v_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'license privacy preflight: owner_merchant_id is missing or drifted';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'file_records'
    AND column_name = 'capability_token_hash'
    AND column_type = 'char(64)'
    AND is_nullable = 'YES';
  IF v_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'license privacy preflight: capability_token_hash is missing or drifted';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'file_records'
    AND column_name = 'capability_expires_at'
    AND data_type = 'datetime'
    AND datetime_precision = 3
    AND is_nullable = 'YES';
  IF v_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'license privacy preflight: capability_expires_at is missing or drifted';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM (
    SELECT index_name
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'file_records'
      AND index_name = 'idx_file_owner_biz_scan'
      AND non_unique = 1
    GROUP BY index_name
    HAVING COUNT(*) = 3
      AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'owner_merchant_id,biz_type,scan_status'
  ) expected_owner_index;
  IF v_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'license privacy preflight: owner/biz/scan index is missing or drifted';
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
      SET MESSAGE_TEXT = 'license privacy preflight: capability token index is missing or drifted';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM (
    SELECT index_name
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'file_records'
      AND index_name = 'idx_file_capability_expires'
      AND non_unique = 1
    GROUP BY index_name
    HAVING COUNT(*) = 1
      AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'capability_expires_at'
  ) expected_expiry_index;
  IF v_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'license privacy preflight: capability expiry index is missing or drifted';
  END IF;

  SELECT COUNT(*) INTO v_bad_license
  FROM file_records
  WHERE biz_type = 'MERCHANT_LICENSE'
    AND (COALESCE(object_key, '') = ''
      OR mime_type NOT IN ('image/jpeg', 'image/png', 'image/webp', 'image/heic', 'image/heif')
      OR scan_status NOT IN ('PENDING', 'PASS', 'BLOCKED'));
  IF v_bad_license <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'license privacy preflight: invalid merchant license record';
  END IF;

  SELECT COUNT(*) INTO v_bad_bound_license
  FROM merchants m
  LEFT JOIN file_records f ON f.id = m.license_file_id
  LEFT JOIN merchant_accounts ma ON ma.id = f.uploader_id
  WHERE m.license_file_id IS NOT NULL
    AND (f.id IS NULL
      OR f.biz_type <> 'MERCHANT_LICENSE'
      OR f.scan_status <> 'PASS'
      OR COALESCE(f.object_key, '') = ''
      OR f.owner_merchant_id IS NULL
      OR f.owner_merchant_id <> m.id
      OR (f.uploader_type = 'MERCHANT' AND (ma.id IS NULL OR ma.merchant_id <> m.id)));
  IF v_bad_bound_license <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'license privacy preflight: invalid bound merchant license';
  END IF;
END//
DELIMITER ;

CALL license_file_privacy_preflight();
DROP PROCEDURE license_file_privacy_preflight;

SELECT 'license_file_privacy_preflight_passed' AS migration_gate;
