-- Issue #16 / F-03 preflight.
-- ISOLATED ACCEPTANCE ONLY until Issue #17 / F-07 is released. Passing this
-- read-only check does not authorize applying 0004 to an active environment.
-- Run only while API and maintenance writers are stopped. Each scalar guard
-- returns one row on success and raises MySQL error 1242 on drift.

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
        FROM information_schema.TABLES
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME IN ('products', 'orders')
      ) <> 2
      OR (
        SELECT COUNT(*)
        FROM information_schema.TABLES
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME IN ('products', 'orders')
          AND TABLE_TYPE = 'BASE TABLE'
          AND UPPER(ENGINE) = 'INNODB'
      ) <> 2
  ) AS f03_table_guard
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
          AND COLUMN_NAME = 'stock'
          AND DATA_TYPE = 'int'
          AND COLUMN_TYPE = 'int'
          AND IS_NULLABLE = 'NO'
          AND COLUMN_DEFAULT = '1'
      ) <> 1
      OR (
        SELECT COUNT(*)
        FROM information_schema.COLUMNS
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME = 'products'
          AND COLUMN_NAME = 'status'
          AND DATA_TYPE = 'varchar'
          AND CHARACTER_MAXIMUM_LENGTH = 16
          AND IS_NULLABLE = 'NO'
      ) <> 1
      OR (
        SELECT COUNT(*)
        FROM information_schema.COLUMNS
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME = 'products'
          AND COLUMN_NAME = 'active_order_id'
          AND DATA_TYPE = 'bigint'
          AND COLUMN_TYPE = 'bigint'
          AND IS_NULLABLE = 'YES'
          AND COLUMN_DEFAULT IS NULL
      ) <> 1
      OR (
        SELECT COUNT(*)
        FROM information_schema.COLUMNS
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME = 'products'
          AND COLUMN_NAME = 'locked_at'
          AND DATA_TYPE = 'datetime'
          AND IS_NULLABLE = 'YES'
          AND COLUMN_DEFAULT IS NULL
      ) <> 1
      OR (
        SELECT COUNT(*)
        FROM information_schema.COLUMNS
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME = 'orders'
          AND COLUMN_NAME = 'product_id'
          AND DATA_TYPE = 'bigint'
          AND COLUMN_TYPE = 'bigint'
          AND IS_NULLABLE = 'NO'
          AND COLUMN_DEFAULT IS NULL
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
          AND COLUMN_NAME = 'is_active'
          AND DATA_TYPE = 'tinyint'
          AND COLUMN_TYPE = 'tinyint(1)'
          AND IS_NULLABLE = 'NO'
          AND COLUMN_DEFAULT IS NULL
      ) <> 1
  ) AS f03_column_guard
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
          AND (
            (TABLE_NAME = 'products' AND COLUMN_NAME = 'reserved_stock')
            OR (TABLE_NAME = 'orders' AND COLUMN_NAME = 'quantity')
          )
      ) <> 0
      OR (
        SELECT COUNT(*)
        FROM information_schema.TABLE_CONSTRAINTS
        WHERE CONSTRAINT_SCHEMA = DATABASE()
          AND CONSTRAINT_NAME IN (
            'chk_products_stock_reservation_bounds',
            'chk_orders_quantity_positive'
          )
      ) <> 0
  ) AS f03_target_absence_guard
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
      ) <> 2
      OR (
        SELECT COUNT(*)
        FROM information_schema.STATISTICS
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME = 'orders'
          AND INDEX_NAME = 'uk_product_active'
          AND NON_UNIQUE = 0
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
        FROM information_schema.STATISTICS
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME = 'orders'
          AND INDEX_NAME = 'idx_product_active'
      ) <> 0
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
      ) <> 1
  ) AS f03_order_index_guard
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
          AND INDEX_NAME = 'idx_product_id'
          AND NON_UNIQUE = 1
          AND INDEX_TYPE = 'BTREE'
          AND IS_VISIBLE = 'YES'
          AND SEQ_IN_INDEX = 1
          AND COLUMN_NAME = 'product_id'
          AND SUB_PART IS NULL
          AND EXPRESSION IS NULL
          AND COLLATION = 'A'
      ) <> 1
      OR (
        SELECT COUNT(*)
        FROM information_schema.STATISTICS
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME = 'products'
          AND INDEX_NAME = 'idx_active_order'
          AND NON_UNIQUE = 1
          AND INDEX_TYPE = 'BTREE'
          AND IS_VISIBLE = 'YES'
          AND SEQ_IN_INDEX = 1
          AND COLUMN_NAME = 'active_order_id'
          AND SUB_PART IS NULL
          AND EXPRESSION IS NULL
          AND COLLATION = 'A'
      ) <> 1
  ) AS f03_preserved_index_guard
);

SET @f03_guard = (
  SELECT guard_value
  FROM (
    SELECT 1 AS guard_value
    UNION ALL
    SELECT 2
    WHERE
      (SELECT COUNT(*) FROM orders WHERE is_active NOT IN (0, 1)) <> 0
      OR (SELECT COUNT(*) FROM orders WHERE is_active = 1) <> 0
      OR (
        SELECT COUNT(*)
        FROM products
        WHERE BINARY status NOT IN (
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
      OR (SELECT COUNT(*) FROM products WHERE stock < 0) <> 0
  ) AS f03_data_guard
);
