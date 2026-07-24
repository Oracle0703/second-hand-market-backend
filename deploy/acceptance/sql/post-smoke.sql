-- Run after all acceptance smoke tests and again after AUTO_MIGRATE=true restart.

DROP PROCEDURE IF EXISTS acceptance_post_smoke;

DELIMITER //
CREATE PROCEDURE acceptance_post_smoke()
BEGIN
  DECLARE failures BIGINT DEFAULT 0;

  SELECT COUNT(*) INTO failures
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'products'
    AND column_name = 'reserved_stock'
    AND data_type = 'int'
    AND is_nullable = 'NO'
    AND column_default = '0';
  IF failures <> 1 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-smoke: products.reserved_stock is missing or drifted';
  END IF;

  SELECT COUNT(*) INTO failures
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'orders'
    AND column_name = 'quantity'
    AND data_type = 'int'
    AND is_nullable = 'NO'
    AND column_default = '1';
  IF failures <> 1 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-smoke: orders.quantity is missing or drifted';
  END IF;

  SELECT COUNT(*) INTO failures
  FROM products
  WHERE stock < 0 OR reserved_stock < 0 OR reserved_stock > stock;
  IF failures <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-smoke: product inventory invariant failed';
  END IF;

  SELECT COUNT(*) INTO failures FROM orders WHERE quantity <= 0;
  IF failures <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-smoke: non-positive order quantity exists';
  END IF;

  SELECT COUNT(*) INTO failures
  FROM orders
  WHERE (is_active = 1 AND status <> 'CREATED')
     OR (is_active = 0 AND status = 'CREATED');
  IF failures <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-smoke: order active flag and status disagree';
  END IF;

  SELECT COUNT(*) INTO failures
  FROM (
    SELECT p.id
    FROM products AS p
    LEFT JOIN (
      SELECT product_id, SUM(quantity) AS active_quantity
      FROM orders
      WHERE is_active = 1 AND deleted_at IS NULL
      GROUP BY product_id
    ) AS active_orders ON active_orders.product_id = p.id
    WHERE p.deleted_at IS NULL
      AND p.reserved_stock <> COALESCE(active_orders.active_quantity, 0)
  ) reservation_mismatches;
  IF failures <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-smoke: reserved stock does not equal active order quantity';
  END IF;

  SELECT COUNT(*) INTO failures FROM orders WHERE is_active = 1;
  IF failures <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-smoke: test cleanup left active orders';
  END IF;

  SELECT COUNT(*) INTO failures FROM products WHERE reserved_stock <> 0;
  IF failures <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-smoke: test cleanup left reserved inventory';
  END IF;

  SELECT COUNT(*) INTO failures FROM products WHERE active_order_id IS NOT NULL;
  IF failures <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-smoke: deprecated active_order_id was written';
  END IF;

  SELECT COUNT(*) INTO failures FROM products WHERE status = 'LOCKED';
  IF failures <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-smoke: deprecated LOCKED state was written';
  END IF;

  SELECT COUNT(*) INTO failures
  FROM information_schema.statistics
  WHERE table_schema = DATABASE()
    AND table_name = 'orders'
    AND index_name = 'uk_product_active';
  IF failures <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-smoke: AutoMigrate recreated obsolete unique index';
  END IF;

  SELECT COUNT(*) INTO failures
  FROM (
    SELECT index_name, non_unique AS is_non_unique
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'orders'
      AND index_name = 'idx_order_product_active'
    GROUP BY index_name, non_unique
    HAVING is_non_unique = 1
      AND COUNT(*) = 2
      AND GROUP_CONCAT(column_name ORDER BY seq_in_index SEPARATOR ',') = 'product_id,is_active'
      AND GROUP_CONCAT(seq_in_index ORDER BY seq_in_index SEPARATOR ',') = '1,2'
  ) expected_ordinary_index;
  IF failures <> 1 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-smoke: ordinary product/active index drifted';
  END IF;

  SELECT COUNT(*) INTO failures
  FROM information_schema.table_constraints AS tc
  INNER JOIN information_schema.check_constraints AS cc
    ON cc.constraint_schema = tc.constraint_schema
   AND cc.constraint_name = tc.constraint_name
  WHERE tc.constraint_schema = DATABASE()
    AND tc.constraint_type = 'CHECK'
    AND tc.enforced = 'YES'
    AND (
      (tc.table_name = 'products'
        AND tc.constraint_name = 'chk_products_reserved_stock_nonnegative'
        AND LOWER(REPLACE(REPLACE(REPLACE(REPLACE(cc.check_clause, '`', ''), ' ', ''), '(', ''), ')', '')) = 'reserved_stock>=0')
      OR (tc.table_name = 'products'
        AND tc.constraint_name = 'chk_products_reserved_stock_lte_stock'
        AND LOWER(REPLACE(REPLACE(REPLACE(REPLACE(cc.check_clause, '`', ''), ' ', ''), '(', ''), ')', '')) = 'reserved_stock<=stock')
      OR (tc.table_name = 'orders'
        AND tc.constraint_name = 'chk_orders_quantity_positive'
        AND LOWER(REPLACE(REPLACE(REPLACE(REPLACE(cc.check_clause, '`', ''), ' ', ''), '(', ''), ')', '')) = 'quantity>0')
    );
  IF failures <> 3 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-smoke: required CHECK constraints are absent, unenforced, or drifted';
  END IF;

  SELECT COUNT(*) INTO failures
  FROM merchant_accounts
  WHERE username = 'yaner' AND deleted_at IS NULL;
  IF failures <> 1 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-smoke: protected yaner account changed unexpectedly';
  END IF;
END//
DELIMITER ;

CALL acceptance_post_smoke();
DROP PROCEDURE acceptance_post_smoke;

SELECT 'post_smoke_passed' AS acceptance_gate, NOW() AS checked_at;
SELECT COUNT(*) AS active_orders FROM orders WHERE is_active = 1;
SELECT COUNT(*) AS products, SUM(stock) AS total_stock, SUM(reserved_stock) AS total_reserved_stock
FROM products WHERE deleted_at IS NULL;
SELECT id, merchant_id, role, status, SHA2(password_hash, 256) AS password_hash_fingerprint
FROM merchant_accounts
WHERE username = 'yaner' AND deleted_at IS NULL;
