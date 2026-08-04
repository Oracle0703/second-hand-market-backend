CREATE TABLE IF NOT EXISTS product_stock_adjustments (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  product_id BIGINT NOT NULL,
  merchant_id BIGINT NOT NULL,
  adjustment_type VARCHAR(32) NOT NULL,
  quantity INT NOT NULL,
  stock_before INT NOT NULL,
  stock_after INT NOT NULL,
  status_before VARCHAR(16) NOT NULL,
  status_after VARCHAR(16) NOT NULL,
  reason VARCHAR(255) NOT NULL,
  operator_id BIGINT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_product_stock_adjustment_created (product_id, created_at),
  INDEX idx_merchant_stock_adjustment_created (merchant_id, created_at)
);
