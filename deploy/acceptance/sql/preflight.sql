-- Run only against the isolated acceptance clone before migration 0004.

DROP PROCEDURE IF EXISTS acceptance_preflight;

DELIMITER //
CREATE PROCEDURE acceptance_preflight()
BEGIN
  DECLARE failures BIGINT DEFAULT 0;

  SELECT COUNT(*) INTO failures
  FROM information_schema.tables
  WHERE table_schema = DATABASE()
    AND table_name IN ('products', 'orders', 'merchant_accounts');
  IF failures <> 3 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'preflight: required production tables are missing';
  END IF;

  SELECT COUNT(*) INTO failures
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND ((table_name = 'products' AND column_name = 'reserved_stock')
      OR (table_name = 'orders' AND column_name = 'quantity'));
  IF failures <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'preflight: migration 0004 appears partially or fully applied';
  END IF;

  SELECT COUNT(*) INTO failures FROM orders WHERE is_active = 1;
  IF failures <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'preflight: active orders must be zero before migration';
  END IF;

  SELECT COUNT(*) INTO failures FROM products WHERE stock < 0;
  IF failures <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'preflight: negative product stock exists';
  END IF;

  SELECT COUNT(*) INTO failures
  FROM (
    SELECT order_no
    FROM orders
    GROUP BY order_no
    HAVING COUNT(*) > 1
  ) duplicate_orders;
  IF failures <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'preflight: duplicate order numbers exist';
  END IF;

  SELECT COUNT(*) INTO failures
  FROM orders AS o
  LEFT JOIN products AS p ON p.id = o.product_id
  WHERE p.id IS NULL;
  IF failures <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'preflight: orphan orders exist';
  END IF;

  SELECT COUNT(*) INTO failures FROM products WHERE status = 'LOCKED';
  IF failures <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'preflight: LOCKED products require manual disposition';
  END IF;

  SELECT COUNT(*) INTO failures
  FROM (
    SELECT index_name, non_unique AS is_non_unique
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'orders'
      AND index_name = 'uk_product_active'
    GROUP BY index_name, non_unique
    HAVING is_non_unique = 0
      AND COUNT(*) = 2
      AND GROUP_CONCAT(column_name ORDER BY seq_in_index SEPARATOR ',') = 'product_id,is_active'
      AND GROUP_CONCAT(seq_in_index ORDER BY seq_in_index SEPARATOR ',') = '1,2'
  ) expected_unique_index;
  IF failures <> 1 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'preflight: expected composite unique index uk_product_active is absent or drifted';
  END IF;

  SELECT COUNT(*) INTO failures
  FROM merchant_accounts
  WHERE username = 'yaner' AND deleted_at IS NULL;
  IF failures <> 1 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'preflight: protected yaner account is absent or duplicated';
  END IF;
END//
DELIMITER ;

CALL acceptance_preflight();
DROP PROCEDURE acceptance_preflight;

SELECT 'preflight_passed' AS acceptance_gate, NOW() AS checked_at, VERSION() AS mysql_version;
SELECT COUNT(*) AS products, SUM(stock) AS total_stock FROM products WHERE deleted_at IS NULL;
SELECT COUNT(*) AS orders, COALESCE(SUM(deal_price_cent), 0) AS unit_price_sum FROM orders WHERE deleted_at IS NULL;
SELECT COUNT(*) AS stale_active_order_ids FROM products WHERE active_order_id IS NOT NULL;
SELECT COUNT(*) AS sold_with_stock FROM products WHERE status = 'SOLD' AND stock > 0;
SELECT id, merchant_id, role, status, SHA2(password_hash, 256) AS password_hash_fingerprint
FROM merchant_accounts
WHERE username = 'yaner' AND deleted_at IS NULL;
