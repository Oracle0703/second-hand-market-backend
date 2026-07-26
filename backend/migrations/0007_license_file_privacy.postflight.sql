DROP PROCEDURE IF EXISTS license_file_privacy_postflight;

DELIMITER //
CREATE PROCEDURE license_file_privacy_postflight()
BEGIN
  DECLARE v_count BIGINT DEFAULT 0;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.tables
  WHERE table_schema = DATABASE() AND table_name = 'file_records';
  IF v_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'license privacy postflight: canonical file_records table is required';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.tables
  WHERE table_schema = DATABASE() AND table_name = 'files';
  IF v_count <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'license privacy postflight: legacy files table remains';
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
      SET MESSAGE_TEXT = 'license privacy postflight: ownership columns are missing';
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
      SET MESSAGE_TEXT = 'license privacy postflight: owner/biz/scan index is missing or drifted';
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
      SET MESSAGE_TEXT = 'license privacy postflight: capability token index is missing or drifted';
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
      SET MESSAGE_TEXT = 'license privacy postflight: capability expiry index is missing or drifted';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM file_records
  WHERE biz_type = 'MERCHANT_LICENSE' AND COALESCE(url, '') <> '';
  IF v_count <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'license privacy postflight: merchant license URL remains public';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM file_records
  WHERE biz_type = 'MERCHANT_LICENSE'
    AND LEFT(object_key, 17) <> 'merchant_license/';
  IF v_count <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'license privacy postflight: merchant license has a public object key prefix';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM file_records
  WHERE biz_type = 'PRODUCT_IMAGE'
    AND scan_status = 'PASS'
    AND COALESCE(url, '') = '';
  IF v_count <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'license privacy postflight: completed product image URL is empty';
  END IF;
END//
DELIMITER ;

CALL license_file_privacy_postflight();
DROP PROCEDURE license_file_privacy_postflight;

SELECT COUNT(*) AS file_record_count FROM file_records;
SELECT COUNT(*) AS merchant_license_count
FROM file_records WHERE biz_type = 'MERCHANT_LICENSE';
SELECT 'license_file_privacy_postflight_passed' AS migration_gate;
