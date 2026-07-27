DROP PROCEDURE IF EXISTS anonymous_upload_governance_preflight;

DELIMITER //
CREATE PROCEDURE anonymous_upload_governance_preflight()
main: BEGIN
  DECLARE v_count BIGINT DEFAULT 0;
  DECLARE v_columns BIGINT DEFAULT 0;
  DECLARE v_indexes BIGINT DEFAULT 0;
  DECLARE v_guard_tables BIGINT DEFAULT 0;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.tables
  WHERE table_schema = DATABASE() AND table_name = 'file_records';
  IF v_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'upload governance preflight: canonical file_records table is required';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.tables
  WHERE table_schema = DATABASE()
    AND table_name = 'file_records'
    AND engine = 'InnoDB';
  IF v_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'upload governance preflight: file_records must use InnoDB';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.tables
  WHERE table_schema = DATABASE() AND table_name = 'files';
  IF v_count <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'upload governance preflight: legacy files table must not exist';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'file_records'
    AND (
      (column_name = 'owner_merchant_id' AND column_type = 'bigint unsigned' AND is_nullable = 'YES')
      OR (column_name = 'capability_token_hash' AND column_type = 'char(64)' AND is_nullable = 'YES')
      OR (column_name = 'capability_expires_at' AND data_type = 'datetime' AND datetime_precision = 3 AND is_nullable = 'YES')
    );
  IF v_count <> 3 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'upload governance preflight: 0006 ownership columns are missing or drifted';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM (
    SELECT index_name, non_unique AS is_non_unique
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
      SET MESSAGE_TEXT = 'upload governance preflight: 0006 owner index is missing or drifted';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM (
    SELECT index_name
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'file_records'
      AND index_name IN ('uk_file_capability_token', 'idx_file_capability_expires')
    GROUP BY index_name, non_unique
    HAVING (index_name = 'uk_file_capability_token'
        AND is_non_unique = 0
        AND COUNT(*) = 1
        AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'capability_token_hash')
      OR (index_name = 'idx_file_capability_expires'
        AND is_non_unique = 1
        AND COUNT(*) = 1
        AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'capability_expires_at')
  ) expected_capability_indexes;
  IF v_count <> 2 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'upload governance preflight: 0006 capability indexes are missing or drifted';
  END IF;

  SELECT COUNT(*) INTO v_count FROM file_records WHERE size_bytes < 0;
  IF v_count <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'upload governance preflight: negative file size exists';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM file_records
  WHERE biz_type = 'MERCHANT_LICENSE' AND COALESCE(url, '') <> '';
  IF v_count <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'upload governance preflight: 0007 merchant license URL remains public';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM file_records
  WHERE biz_type = 'MERCHANT_LICENSE'
    AND LEFT(object_key, 17) <> 'merchant_license/';
  IF v_count <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'upload governance preflight: 0007 merchant license object key is invalid';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM file_records
  WHERE biz_type = 'PRODUCT_IMAGE'
    AND scan_status = 'PASS'
    AND COALESCE(url, '') = '';
  IF v_count <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'upload governance preflight: 0007 completed product image URL is empty';
  END IF;

  SELECT COUNT(*) INTO v_columns
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'file_records'
    AND column_name IN (
      'source_ip_hash', 'cleanup_after', 'cleanup_claimed_at',
      'cleanup_claim_token', 'cleanup_attempts'
    );

  SELECT COUNT(DISTINCT index_name) INTO v_indexes
  FROM information_schema.statistics
  WHERE table_schema = DATABASE()
    AND table_name = 'file_records'
    AND index_name IN ('idx_file_source_created', 'idx_file_cleanup_candidate');

  SELECT COUNT(*) INTO v_guard_tables
  FROM information_schema.tables
  WHERE table_schema = DATABASE() AND table_name = 'file_quota_guards';

  IF v_columns + v_indexes + v_guard_tables = 0 THEN
    SELECT 'anonymous_upload_governance_preflight_passed' AS migration_gate;
    LEAVE main;
  END IF;

  IF v_columns <> 5 OR v_indexes <> 2 OR v_guard_tables <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'upload governance preflight: partial 0008 schema exists';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.tables
  WHERE table_schema = DATABASE()
    AND table_name = 'file_quota_guards'
    AND engine = 'InnoDB';
  IF v_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'upload governance preflight: quota guard table must use InnoDB';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'file_records'
    AND (
      (column_name = 'source_ip_hash' AND column_type = 'char(64)' AND is_nullable = 'YES')
      OR (column_name = 'cleanup_after' AND data_type = 'datetime' AND datetime_precision = 3 AND is_nullable = 'YES')
      OR (column_name = 'cleanup_claimed_at' AND data_type = 'datetime' AND datetime_precision = 3 AND is_nullable = 'YES')
      OR (column_name = 'cleanup_claim_token' AND column_type = 'char(64)' AND is_nullable = 'YES')
      OR (column_name = 'cleanup_attempts' AND column_type = 'int unsigned' AND is_nullable = 'NO' AND column_default = '0')
    );
  IF v_count <> 5 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'upload governance preflight: 0008 columns are drifted';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM (
    SELECT index_name, non_unique AS is_non_unique
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'file_records'
      AND index_name IN ('idx_file_source_created', 'idx_file_cleanup_candidate')
      AND non_unique = 1
    GROUP BY index_name
    HAVING (index_name = 'idx_file_source_created'
        AND COUNT(*) = 2
        AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'source_ip_hash,created_at')
      OR (index_name = 'idx_file_cleanup_candidate'
        AND COUNT(*) = 4
        AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'uploader_type,owner_merchant_id,cleanup_after,cleanup_claimed_at')
  ) expected_governance_indexes;
  IF v_count <> 2 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'upload governance preflight: 0008 indexes are drifted';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'file_quota_guards'
    AND (
      (column_name = 'id' AND column_type = 'tinyint unsigned' AND is_nullable = 'NO')
      OR (column_name = 'guard_name' AND column_type = 'varchar(32)' AND is_nullable = 'NO')
      OR (column_name = 'created_at' AND data_type = 'datetime' AND datetime_precision = 3 AND is_nullable = 'NO')
    );
  IF v_count <> 3 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'upload governance preflight: quota guard columns are drifted';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM (
    SELECT index_name
    FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'file_quota_guards'
    GROUP BY index_name, non_unique
    HAVING (index_name = 'PRIMARY' AND is_non_unique = 0 AND COUNT(*) = 1
        AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'id')
      OR (index_name = 'uk_file_quota_guard_name' AND is_non_unique = 0 AND COUNT(*) = 1
        AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'guard_name')
  ) expected_guard_indexes;
  IF v_count <> 2 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'upload governance preflight: quota guard indexes are drifted';
  END IF;

  SELECT COUNT(*) INTO v_count FROM file_quota_guards;
  IF v_count <> 1 OR NOT EXISTS (
    SELECT 1 FROM file_quota_guards WHERE id = 1 AND guard_name = 'file_records'
  ) THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'upload governance preflight: fixed quota guard row is missing or drifted';
  END IF;

  SELECT 'anonymous_upload_governance_preflight_passed' AS migration_gate;
END//
DELIMITER ;

CALL anonymous_upload_governance_preflight();
DROP PROCEDURE anonymous_upload_governance_preflight;
