DROP PROCEDURE IF EXISTS license_file_privacy_migration;

DELIMITER //
CREATE PROCEDURE license_file_privacy_migration()
BEGIN
  DECLARE v_count BIGINT DEFAULT 0;

  SELECT COUNT(*) INTO v_count
  FROM information_schema.tables
  WHERE table_schema = DATABASE() AND table_name = 'file_records';
  IF v_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'license privacy migration: canonical file_records table is required';
  END IF;

  UPDATE file_records
  SET url = ''
  WHERE biz_type = 'MERCHANT_LICENSE' AND url <> '';

  SELECT 'license_file_privacy_migration_applied' AS migration_gate;
END//
DELIMITER ;

CALL license_file_privacy_migration();
DROP PROCEDURE license_file_privacy_migration;
