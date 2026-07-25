DROP PROCEDURE IF EXISTS merchant_multi_stock_postflight;

DELIMITER //
CREATE PROCEDURE merchant_multi_stock_postflight()
BEGIN
  DECLARE failures BIGINT DEFAULT 0;

  SELECT COUNT(*) INTO failures
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'products'
    AND column_name = 'reserved_stock'
    AND column_type = 'int'
    AND is_nullable = 'NO'
    AND column_default = '0';
  IF failures <> 1 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-migration: products.reserved_stock is missing or drifted';
  END IF;

  SELECT COUNT(*) INTO failures
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'orders'
    AND column_name = 'quantity'
    AND column_type = 'int'
    AND is_nullable = 'NO'
    AND column_default = '1';
  IF failures <> 1 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-migration: orders.quantity is missing or drifted';
  END IF;

  SELECT COUNT(*) INTO failures FROM orders WHERE quantity <> 1 OR quantity IS NULL;
  IF failures <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-migration: historical order quantity backfill failed';
  END IF;

  SELECT COUNT(*) INTO failures
  FROM products
  WHERE reserved_stock <> 0 OR reserved_stock IS NULL OR reserved_stock > stock;
  IF failures <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-migration: reserved stock backfill or invariant failed';
  END IF;

  SELECT COUNT(*) INTO failures FROM orders WHERE is_active = 1;
  IF failures <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-migration: active orders unexpectedly exist';
  END IF;

  SELECT COUNT(*) INTO failures FROM products WHERE active_order_id IS NOT NULL;
  IF failures <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-migration: active_order_id was not cleared';
  END IF;

  SELECT COUNT(*) INTO failures FROM products WHERE status = 'LOCKED';
  IF failures <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-migration: LOCKED products remain';
  END IF;

  SELECT COUNT(*) INTO failures
  FROM information_schema.statistics
  WHERE table_schema = DATABASE()
    AND table_name = 'orders'
    AND index_name = 'uk_product_active';
  IF failures <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-migration: obsolete unique index still exists';
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
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-migration: ordinary product/active index is absent or drifted';
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
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'post-migration: required CHECK constraints are absent, unenforced, or drifted';
  END IF;
END//
DELIMITER ;

CALL merchant_multi_stock_postflight();
DROP PROCEDURE merchant_multi_stock_postflight;

SELECT 'postflight_passed' AS migration_gate;
SELECT COUNT(*) AS products, SUM(stock) AS total_stock, SUM(reserved_stock) AS total_reserved_stock
FROM products WHERE deleted_at IS NULL;
SELECT COUNT(*) AS orders, SUM(quantity) AS total_quantity, COALESCE(SUM(deal_price_cent), 0) AS unit_price_sum
FROM orders WHERE deleted_at IS NULL;
