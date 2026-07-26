DROP PROCEDURE IF EXISTS anonymous_upload_governance_migration;

DELIMITER //
CREATE PROCEDURE anonymous_upload_governance_migration()
BEGIN
  DECLARE v_count BIGINT DEFAULT 0;
  DECLARE v_columns BIGINT DEFAULT 0;
  DECLARE v_indexes BIGINT DEFAULT 0;
  DECLARE v_guard_tables BIGINT DEFAULT 0;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.tables
  WHERE table_schema = DATABASE() AND table_name = 'file_records';
  IF v_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'upload governance migration: canonical file_records table is required';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.tables
  WHERE table_schema = DATABASE()
    AND table_name = 'file_records'
    AND engine = 'InnoDB';
  IF v_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'upload governance migration: file_records must use InnoDB';
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
    ALTER TABLE file_records
      ADD COLUMN source_ip_hash CHAR(64) NULL,
      ADD COLUMN cleanup_after DATETIME(3) NULL,
      ADD COLUMN cleanup_claimed_at DATETIME(3) NULL,
      ADD COLUMN cleanup_claim_token CHAR(64) NULL,
      ADD COLUMN cleanup_attempts INT UNSIGNED NOT NULL DEFAULT 0,
      ADD INDEX idx_file_source_created (source_ip_hash, created_at),
      ADD INDEX idx_file_cleanup_candidate
        (uploader_type, owner_merchant_id, cleanup_after, cleanup_claimed_at);

    CREATE TABLE file_quota_guards (
      id TINYINT UNSIGNED NOT NULL,
      guard_name VARCHAR(32) NOT NULL,
      created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
      PRIMARY KEY (id),
      UNIQUE KEY uk_file_quota_guard_name (guard_name)
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

    INSERT INTO file_quota_guards (id, guard_name) VALUES (1, 'file_records');
  ELSE
    IF v_columns <> 5 OR v_indexes <> 2 OR v_guard_tables <> 1 THEN
      SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'upload governance migration: partial 0008 schema exists';
    END IF;

    SELECT COUNT(*) INTO v_count
    FROM information_schema.tables
    WHERE table_schema = DATABASE()
      AND table_name = 'file_quota_guards'
      AND engine = 'InnoDB';
    IF v_count <> 1 THEN
      SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'upload governance migration: quota guard table must use InnoDB';
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
        SET MESSAGE_TEXT = 'upload governance migration: 0008 columns are drifted';
    END IF;

    SELECT COUNT(*) INTO v_count
    FROM (
      SELECT index_name
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
        SET MESSAGE_TEXT = 'upload governance migration: 0008 indexes are drifted';
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
        SET MESSAGE_TEXT = 'upload governance migration: quota guard columns are drifted';
    END IF;

    SELECT COUNT(*) INTO v_count
    FROM (
      SELECT index_name
      FROM information_schema.statistics
      WHERE table_schema = DATABASE() AND table_name = 'file_quota_guards'
      GROUP BY index_name, non_unique
      HAVING (index_name = 'PRIMARY' AND non_unique = 0 AND COUNT(*) = 1
          AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'id')
        OR (index_name = 'uk_file_quota_guard_name' AND non_unique = 0 AND COUNT(*) = 1
          AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'guard_name')
    ) expected_guard_indexes;
    IF v_count <> 2 THEN
      SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'upload governance migration: quota guard indexes are drifted';
    END IF;

    SELECT COUNT(*) INTO v_count FROM file_quota_guards;
    IF v_count <> 1 OR NOT EXISTS (
      SELECT 1 FROM file_quota_guards WHERE id = 1 AND guard_name = 'file_records'
    ) THEN
      SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'upload governance migration: fixed quota guard row is missing or drifted';
    END IF;
  END IF;

  SELECT 'anonymous_upload_governance_migration_applied' AS migration_gate;
END//
DELIMITER ;

CALL anonymous_upload_governance_migration();
DROP PROCEDURE anonymous_upload_governance_migration;
