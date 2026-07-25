DROP PROCEDURE IF EXISTS file_binding_ownership_preflight;

DELIMITER //
CREATE PROCEDURE file_binding_ownership_preflight()
BEGIN
  DECLARE v_file_records BIGINT DEFAULT 0;
  DECLARE v_files BIGINT DEFAULT 0;
  DECLARE v_bad_product_files BIGINT DEFAULT 0;
  DECLARE v_bad_license_files BIGINT DEFAULT 0;
  DECLARE v_cross_merchant_files BIGINT DEFAULT 0;
  DECLARE v_uploader_mismatches BIGINT DEFAULT 0;

  SELECT COUNT(*) INTO v_file_records
  FROM information_schema.tables
  WHERE table_schema = DATABASE() AND table_name = 'file_records';

  SELECT COUNT(*) INTO v_files
  FROM information_schema.tables
  WHERE table_schema = DATABASE() AND table_name = 'files';

  IF v_file_records <> 1 OR v_files <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file binding preflight: canonical file_records table is required';
  END IF;

  SELECT COUNT(*) INTO v_bad_product_files
  FROM product_images pi
  JOIN products p ON p.id = pi.product_id
  LEFT JOIN file_records f ON f.id = pi.file_id
  WHERE f.id IS NULL
     OR f.biz_type <> 'PRODUCT_IMAGE'
     OR f.scan_status <> 'PASS'
     OR COALESCE(f.url, '') = '';

  IF v_bad_product_files <> 0 THEN
    SELECT pi.file_id, p.merchant_id, f.biz_type, f.scan_status
    FROM product_images pi
    JOIN products p ON p.id = pi.product_id
    LEFT JOIN file_records f ON f.id = pi.file_id
    WHERE f.id IS NULL
       OR f.biz_type <> 'PRODUCT_IMAGE'
       OR f.scan_status <> 'PASS'
       OR COALESCE(f.url, '') = '';
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file binding preflight: invalid product image references';
  END IF;

  SELECT COUNT(*) INTO v_bad_license_files
  FROM merchants m
  LEFT JOIN file_records f ON f.id = m.license_file_id
  WHERE m.license_file_id IS NOT NULL
    AND (f.id IS NULL
      OR f.biz_type <> 'MERCHANT_LICENSE'
      OR f.scan_status <> 'PASS'
      OR COALESCE(f.url, '') = '');

  IF v_bad_license_files <> 0 THEN
    SELECT m.license_file_id AS file_id, m.id AS merchant_id,
           f.biz_type, f.scan_status
    FROM merchants m
    LEFT JOIN file_records f ON f.id = m.license_file_id
    WHERE m.license_file_id IS NOT NULL
      AND (f.id IS NULL
        OR f.biz_type <> 'MERCHANT_LICENSE'
        OR f.scan_status <> 'PASS'
        OR COALESCE(f.url, '') = '');
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file binding preflight: invalid merchant license references';
  END IF;

  SELECT COUNT(*) INTO v_cross_merchant_files
  FROM (
    SELECT refs.file_id
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
    HAVING COUNT(DISTINCT merchant_id) > 1
  ) conflicts;

  IF v_cross_merchant_files <> 0 THEN
    SELECT refs.file_id, GROUP_CONCAT(DISTINCT refs.merchant_id ORDER BY refs.merchant_id) AS merchant_ids
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
    HAVING COUNT(DISTINCT merchant_id) > 1;
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file binding preflight: file is referenced by multiple merchants';
  END IF;

  SELECT COUNT(*) INTO v_uploader_mismatches
  FROM (
    SELECT pi.file_id, p.merchant_id
    FROM product_images pi
    JOIN products p ON p.id = pi.product_id
    UNION
    SELECT m.license_file_id AS file_id, m.id AS merchant_id
    FROM merchants m
    WHERE m.license_file_id IS NOT NULL
  ) refs
  JOIN file_records f ON f.id = refs.file_id
  LEFT JOIN merchant_accounts ma ON ma.id = f.uploader_id
  WHERE f.uploader_type = 'MERCHANT'
    AND (ma.id IS NULL OR ma.merchant_id <> refs.merchant_id);

  IF v_uploader_mismatches <> 0 THEN
    SELECT refs.file_id, refs.merchant_id, f.uploader_id,
           ma.merchant_id AS uploader_merchant_id
    FROM (
      SELECT pi.file_id, p.merchant_id
      FROM product_images pi
      JOIN products p ON p.id = pi.product_id
      UNION
      SELECT m.license_file_id AS file_id, m.id AS merchant_id
      FROM merchants m
      WHERE m.license_file_id IS NOT NULL
    ) refs
    JOIN file_records f ON f.id = refs.file_id
    LEFT JOIN merchant_accounts ma ON ma.id = f.uploader_id
    WHERE f.uploader_type = 'MERCHANT'
      AND (ma.id IS NULL OR ma.merchant_id <> refs.merchant_id);
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file binding preflight: merchant uploader ownership mismatch';
  END IF;
END//
DELIMITER ;

CALL file_binding_ownership_preflight();
DROP PROCEDURE file_binding_ownership_preflight;

SELECT 'file_binding_ownership_preflight_passed' AS migration_gate;
