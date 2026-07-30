-- Issue #16 / F-03 limited rollback.
-- This rollback is allowed only before multi-stock data exists. It removes the
-- two new columns and their checks, but deliberately keeps the non-unique
-- idx_product_active index and never recreates uk_product_active.
-- Run only while API and maintenance writers are stopped, under separate
-- rollback authorization. This file is not an alternative to Issue #17.

SET @f03_guard = (
  SELECT guard_value
  FROM (
    SELECT 1 AS guard_value
    UNION ALL
    SELECT 2
    WHERE DATABASE() IS NULL
  ) AS f03_database_guard
);

SET @f03_guard = (
  SELECT guard_value
  FROM (
    SELECT 1 AS guard_value
    UNION ALL
    SELECT 2
    WHERE
      (
        SELECT COUNT(*)
        FROM information_schema.COLUMNS
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME = 'products'
          AND COLUMN_NAME = 'reserved_stock'
          AND DATA_TYPE = 'int'
          AND COLUMN_TYPE = 'int'
          AND IS_NULLABLE = 'NO'
          AND COLUMN_DEFAULT = '0'
      ) <> 1
      OR (
        SELECT COUNT(*)
        FROM information_schema.COLUMNS
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME = 'orders'
          AND COLUMN_NAME = 'status'
          AND DATA_TYPE = 'varchar'
          AND CHARACTER_MAXIMUM_LENGTH = 16
          AND IS_NULLABLE = 'NO'
      ) <> 1
      OR (
        SELECT COUNT(*)
        FROM information_schema.COLUMNS
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME = 'orders'
          AND COLUMN_NAME = 'quantity'
          AND DATA_TYPE = 'int'
          AND COLUMN_TYPE = 'int'
          AND IS_NULLABLE = 'NO'
          AND COLUMN_DEFAULT = '1'
      ) <> 1
      OR (
        SELECT COUNT(*)
        FROM information_schema.TABLE_CONSTRAINTS
        WHERE CONSTRAINT_SCHEMA = DATABASE()
          AND TABLE_NAME = 'products'
          AND CONSTRAINT_NAME =
            'chk_products_stock_reservation_bounds'
          AND CONSTRAINT_TYPE = 'CHECK'
          AND ENFORCED = 'YES'
      ) <> 1
      OR (
        SELECT COUNT(*)
        FROM information_schema.TABLE_CONSTRAINTS
        WHERE CONSTRAINT_SCHEMA = DATABASE()
          AND TABLE_NAME = 'orders'
          AND CONSTRAINT_NAME = 'chk_orders_quantity_positive'
          AND CONSTRAINT_TYPE = 'CHECK'
          AND ENFORCED = 'YES'
      ) <> 1
  ) AS f03_rollback_shape_guard
);

SET @f03_guard = (
  SELECT guard_value
  FROM (
    SELECT 1 AS guard_value
    UNION ALL
    SELECT 2
    WHERE
      (
        SELECT COUNT(*)
        FROM information_schema.STATISTICS
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME = 'orders'
          AND INDEX_NAME = 'uk_product_active'
      ) <> 0
      OR (
        SELECT COUNT(*)
        FROM information_schema.STATISTICS
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME = 'orders'
          AND INDEX_NAME = 'idx_product_active'
      ) <> 2
      OR (
        SELECT COUNT(*)
        FROM information_schema.STATISTICS
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME = 'orders'
          AND INDEX_NAME = 'idx_product_active'
          AND NON_UNIQUE = 1
          AND INDEX_TYPE = 'BTREE'
          AND IS_VISIBLE = 'YES'
          AND SUB_PART IS NULL
          AND EXPRESSION IS NULL
          AND COLLATION = 'A'
          AND (
            (SEQ_IN_INDEX = 1 AND COLUMN_NAME = 'product_id')
            OR (SEQ_IN_INDEX = 2 AND COLUMN_NAME = 'is_active')
          )
      ) <> 2
      OR (
        SELECT COUNT(*)
        FROM (
          SELECT INDEX_NAME
          FROM information_schema.STATISTICS
          WHERE TABLE_SCHEMA = DATABASE()
            AND TABLE_NAME = 'orders'
          GROUP BY INDEX_NAME
          HAVING MIN(NON_UNIQUE) = 0
            AND MAX(NON_UNIQUE) = 0
            AND SUM(
              CASE WHEN COLUMN_NAME = 'product_id' THEN 1 ELSE 0 END
            ) > 0
        ) AS f03_unique_product_indexes
      ) <> 0
  ) AS f03_rollback_index_guard
);

SET @f03_guard = (
  SELECT guard_value
  FROM (
    SELECT 1 AS guard_value
    UNION ALL
    SELECT 2
    WHERE
      (SELECT COUNT(*) FROM orders WHERE quantity <> 1) <> 0
      OR (SELECT COUNT(*) FROM orders WHERE is_active NOT IN (0, 1)) <> 0
      OR (SELECT COUNT(*) FROM orders WHERE is_active = 1) <> 0
      OR (
        SELECT COUNT(*)
        FROM products
        WHERE reserved_stock <> 0
          OR BINARY status NOT IN (
            BINARY 'DRAFT',
            BINARY 'ON_SHELF',
            BINARY 'OFF_SHELF',
            BINARY 'SOLD',
            BINARY 'CLOSED'
          )
          OR active_order_id IS NOT NULL
          OR locked_at IS NOT NULL
      ) <> 0
      OR (
        SELECT COUNT(*)
        FROM orders
        WHERE BINARY status NOT IN (
          BINARY 'COMPLETED',
          BINARY 'CLOSED'
        )
      ) <> 0
  ) AS f03_rollback_data_guard
);

-- Reverse the two-table change in the opposite order. A failure between these
-- atomic ALTER statements leaves a dirty state that every 0004 guard rejects.
ALTER TABLE orders
  DROP CHECK chk_orders_quantity_positive,
  DROP COLUMN quantity;

ALTER TABLE products
  DROP CHECK chk_products_stock_reservation_bounds,
  DROP COLUMN reserved_stock;
