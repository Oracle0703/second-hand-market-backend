DROP PROCEDURE IF EXISTS file_records_preflight;

DELIMITER //
CREATE PROCEDURE file_records_preflight()
BEGIN
  DECLARE files_count BIGINT DEFAULT 0;
  DECLARE file_records_count BIGINT DEFAULT 0;
  DECLARE required_columns BIGINT DEFAULT 0;
  DECLARE primary_keys BIGINT DEFAULT 0;
  DECLARE object_key_uniques BIGINT DEFAULT 0;
  DECLARE biz_created_indexes BIGINT DEFAULT 0;
  DECLARE candidate_table VARCHAR(64);

  SELECT SUM(table_name = 'files'), SUM(table_name = 'file_records')
    INTO files_count, file_records_count
  FROM information_schema.tables
  WHERE table_schema = DATABASE()
    AND table_name IN ('files', 'file_records');

  SET files_count = COALESCE(files_count, 0);
  SET file_records_count = COALESCE(file_records_count, 0);

  IF files_count + file_records_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file records preflight: exactly one of files or file_records must exist';
  END IF;

  SET candidate_table = IF(files_count = 1, 'files', 'file_records');

  SELECT COUNT(*) INTO required_columns
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = candidate_table
    AND column_name IN (
      'id', 'biz_type', 'object_key', 'url', 'mime_type',
      'size_bytes', 'uploader_type', 'uploader_id', 'scan_status', 'created_at'
    );
  IF required_columns <> 10 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file records preflight: required columns are missing';
  END IF;

  SELECT COUNT(*) INTO primary_keys
  FROM (
    SELECT index_name
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = candidate_table
      AND index_name = 'PRIMARY'
    GROUP BY index_name
    HAVING COUNT(*) = 1
      AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'id'
  ) expected_primary;
  IF primary_keys <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file records preflight: primary key on id is missing or drifted';
  END IF;

  SELECT COUNT(*) INTO object_key_uniques
  FROM (
    SELECT index_name
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = candidate_table
      AND non_unique = 0
    GROUP BY index_name
    HAVING COUNT(*) = 1
      AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'object_key'
  ) expected_object_key_unique;
  IF object_key_uniques < 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file records preflight: object_key uniqueness is missing';
  END IF;

  SELECT COUNT(DISTINCT first_column.index_name) INTO biz_created_indexes
  FROM information_schema.statistics AS first_column
  INNER JOIN information_schema.statistics AS second_column
    ON second_column.table_schema = first_column.table_schema
   AND second_column.table_name = first_column.table_name
   AND second_column.index_name = first_column.index_name
   AND second_column.seq_in_index = 2
   AND second_column.column_name = 'created_at'
  WHERE first_column.table_schema = DATABASE()
    AND first_column.table_name = candidate_table
    AND first_column.seq_in_index = 1
    AND first_column.column_name = 'biz_type';
  IF biz_created_indexes < 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file records preflight: biz_type,created_at index is missing';
  END IF;
END//
DELIMITER ;

CALL file_records_preflight();
DROP PROCEDURE file_records_preflight;

SELECT 'file_records_preflight_passed' AS migration_gate;
