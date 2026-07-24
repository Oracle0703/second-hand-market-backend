ALTER TABLE orders
  DROP INDEX idx_order_product_active,
  ADD UNIQUE INDEX uk_product_active (product_id, is_active),
  DROP CHECK chk_orders_quantity_positive,
  DROP COLUMN quantity;

ALTER TABLE products
  DROP CHECK chk_products_reserved_stock_lte_stock,
  DROP CHECK chk_products_reserved_stock_nonnegative,
  DROP COLUMN reserved_stock;
