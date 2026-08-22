ALTER TABLE categories
  DROP INDEX uk_parent_name,
  ADD INDEX idx_category_scope_name (merchant_id, parent_id, level, name);
