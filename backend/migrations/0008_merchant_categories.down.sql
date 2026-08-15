ALTER TABLE categories
  DROP INDEX idx_merchant_parent_sort,
  DROP INDEX idx_merchant_level_status_sort,
  DROP COLUMN merchant_id;
