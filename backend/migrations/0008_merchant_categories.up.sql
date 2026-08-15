ALTER TABLE categories
  ADD COLUMN merchant_id BIGINT NULL AFTER id,
  ADD INDEX idx_merchant_parent_sort (merchant_id, parent_id, sort),
  ADD INDEX idx_merchant_level_status_sort (merchant_id, level, status, sort);
