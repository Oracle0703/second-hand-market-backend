DROP PROCEDURE IF EXISTS buyer_intent_open_uniqueness_postflight;

DELIMITER //
CREATE PROCEDURE buyer_intent_open_uniqueness_postflight()
BEGIN
  DECLARE v_count BIGINT DEFAULT 0;
  DECLARE v_baseline_columns BIGINT DEFAULT 0;
  DECLARE v_marker BIGINT DEFAULT 0;
  DECLARE v_marker_exact BIGINT DEFAULT 0;
  DECLARE v_legacy_key BIGINT DEFAULT 0;
  DECLARE v_legacy_exact BIGINT DEFAULT 0;
  DECLARE v_open_key BIGINT DEFAULT 0;
  DECLARE v_open_exact BIGINT DEFAULT 0;
  DECLARE v_relevant_keys BIGINT DEFAULT 0;
  DECLARE v_relevant_lookalikes BIGINT DEFAULT 0;
  DECLARE v_invalid_rows BIGINT DEFAULT 0;
  DECLARE v_duplicate_groups BIGINT DEFAULT 0;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.tables
  WHERE table_schema = DATABASE()
    AND table_name = 'buyer_intents'
    AND engine = 'InnoDB';
  IF v_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'buyer intent postflight: one InnoDB table is required';
  END IF;

  SELECT COUNT(*) INTO v_baseline_columns
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'buyer_intents'
    AND column_name IN ('id', 'intent_no', 'buyer_id', 'product_id', 'status', 'is_open');
  IF v_baseline_columns <> 6 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'buyer intent postflight: baseline columns are missing';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'buyer_intents'
    AND COALESCE(generation_expression, '') = ''
    AND (
      (column_name = 'id' AND ordinal_position = 1
        AND data_type = 'bigint' AND column_type = 'bigint' AND is_nullable = 'NO')
      OR (column_name = 'intent_no' AND ordinal_position = 2
        AND data_type = 'varchar' AND column_type = 'varchar(32)' AND is_nullable = 'NO')
      OR (column_name = 'buyer_id' AND ordinal_position = 3
        AND data_type = 'bigint' AND column_type = 'bigint' AND is_nullable = 'NO')
      OR (column_name = 'product_id' AND ordinal_position = 5
        AND data_type = 'bigint' AND column_type = 'bigint' AND is_nullable = 'NO')
      OR (column_name = 'status' AND ordinal_position = 7
        AND data_type = 'varchar' AND column_type = 'varchar(16)' AND is_nullable = 'NO')
      OR (column_name = 'is_open' AND ordinal_position = 8
        AND data_type = 'tinyint' AND column_type = 'tinyint(1)' AND is_nullable = 'NO')
    );
  IF v_count <> 6 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'buyer intent postflight: baseline columns are reordered or drifted';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM (
    SELECT index_name
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'buyer_intents'
      AND index_name = 'PRIMARY'
    GROUP BY index_name
    HAVING MIN(non_unique) = 0
      AND MAX(non_unique) = 0
      AND COUNT(*) = 1
      AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'id'
  ) AS expected_primary_key;
  IF v_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'buyer intent postflight: primary key is missing or drifted';
  END IF;

  SELECT COUNT(*) INTO v_count
  FROM (
    SELECT index_name
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'buyer_intents'
      AND index_name <> 'PRIMARY'
    GROUP BY index_name
    HAVING MIN(non_unique) = 0
      AND MAX(non_unique) = 0
      AND COUNT(*) = 1
      AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'intent_no'
  ) AS expected_intent_number_key;
  IF v_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'buyer intent postflight: intent number key is missing or duplicated';
  END IF;

  SELECT COUNT(*) INTO v_marker
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'buyer_intents'
    AND column_name = 'open_marker';
  SELECT COUNT(*) INTO v_marker_exact
  FROM (
    SELECT data_type, column_type, is_nullable, generation_expression, extra,
      CASE
        WHEN generation_expression IS NOT NULL AND generation_expression <> '' THEN 'ALWAYS'
        ELSE 'NEVER'
      END AS is_generated
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'buyer_intents'
      AND column_name = 'open_marker'
  ) AS marker_definition
  WHERE data_type = 'tinyint'
    AND column_type = 'tinyint'
    AND is_nullable = 'YES'
    AND is_generated = 'ALWAYS'
    AND UPPER(extra) LIKE '%STORED GENERATED%'
    AND LOWER(
      REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(
        generation_expression,
        '`', ''), ' ', ''), CHAR(9), ''), CHAR(10), ''), CHAR(13), ''),
        '(', ''), ')', '')
    ) = 'casewhenis_open=1then1elsenullend';
  IF v_marker <> 1 OR v_marker_exact <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'buyer intent postflight: generated marker is missing or drifted';
  END IF;

  SELECT COUNT(DISTINCT index_name) INTO v_legacy_key
  FROM information_schema.statistics
  WHERE table_schema = DATABASE()
    AND table_name = 'buyer_intents'
    AND index_name = 'uk_buyer_product_open';
  SELECT COUNT(*) INTO v_legacy_exact
  FROM (
    SELECT index_name
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'buyer_intents'
      AND index_name = 'uk_buyer_product_open'
    GROUP BY index_name
    HAVING MIN(non_unique) = 0
      AND MAX(non_unique) = 0
      AND COUNT(*) = 3
      AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'buyer_id,product_id,is_open'
  ) AS legacy_key_definition;
  IF v_legacy_key <> 0 OR v_legacy_exact <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'buyer intent postflight: legacy key remains';
  END IF;

  SELECT COUNT(DISTINCT index_name) INTO v_open_key
  FROM information_schema.statistics
  WHERE table_schema = DATABASE()
    AND table_name = 'buyer_intents'
    AND index_name = 'uk_buyer_intent_open';
  SELECT COUNT(*) INTO v_open_exact
  FROM (
    SELECT index_name
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'buyer_intents'
      AND index_name = 'uk_buyer_intent_open'
    GROUP BY index_name
    HAVING MIN(non_unique) = 0
      AND MAX(non_unique) = 0
      AND COUNT(*) = 3
      AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'buyer_id,product_id,open_marker'
  ) AS expected_open_key;
  IF v_open_key <> 1 OR v_open_exact <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'buyer intent postflight: open key is missing or drifted';
  END IF;

  SELECT COUNT(*) INTO v_relevant_keys
  FROM (
    SELECT index_name
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'buyer_intents'
      AND index_name <> 'PRIMARY'
    GROUP BY index_name
    HAVING MIN(non_unique) = 0
      AND MAX(non_unique) = 0
      AND SUM(column_name = 'buyer_id') > 0
      AND SUM(column_name = 'product_id') > 0
  ) AS relevant_unique_keys;
  SET v_relevant_lookalikes = v_relevant_keys - v_open_exact;
  IF v_relevant_keys <> 1 OR v_relevant_lookalikes <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'buyer intent postflight: relevant unique key is drifted';
  END IF;

  SELECT COUNT(*) INTO v_invalid_rows
  FROM buyer_intents
  WHERE CASE
    WHEN status IN ('NEW', 'CONTACTED') AND is_open = 1 THEN 0
    WHEN status = 'CLOSED' AND is_open = 0 THEN 0
    ELSE 1
  END = 1;
  IF v_invalid_rows <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'buyer intent postflight: row state is invalid';
  END IF;

  SELECT COUNT(*) INTO v_duplicate_groups
  FROM (
    SELECT buyer_id, product_id
    FROM buyer_intents
    WHERE is_open = 1
    GROUP BY buyer_id, product_id
    HAVING COUNT(*) > 1
  ) AS duplicate_open_intents;
  IF v_duplicate_groups <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'buyer intent postflight: duplicate open group exists';
  END IF;

  SELECT 'buyer_intent_open_uniqueness_postflight_passed' AS migration_gate;
END//
DELIMITER ;

CALL buyer_intent_open_uniqueness_postflight();
DROP PROCEDURE buyer_intent_open_uniqueness_postflight;
