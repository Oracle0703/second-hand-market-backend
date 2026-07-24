ALTER TABLE products
  ADD COLUMN reserved_stock INT NOT NULL DEFAULT 0 AFTER stock,
  ADD CONSTRAINT chk_products_reserved_stock_nonnegative CHECK (reserved_stock >= 0),
  ADD CONSTRAINT chk_products_reserved_stock_lte_stock CHECK (reserved_stock <= stock);

ALTER TABLE orders
  ADD COLUMN quantity INT NOT NULL DEFAULT 1 AFTER product_id,
  ADD CONSTRAINT chk_orders_quantity_positive CHECK (quantity > 0);

UPDATE orders SET quantity = 1 WHERE quantity IS NULL OR quantity <= 0;
UPDATE products SET reserved_stock = 0, active_order_id = NULL;

ALTER TABLE orders
  DROP INDEX uk_product_active,
  ADD INDEX idx_order_product_active (product_id, is_active);
