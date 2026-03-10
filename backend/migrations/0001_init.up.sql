CREATE TABLE IF NOT EXISTS merchants (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  merchant_no VARCHAR(32) NOT NULL UNIQUE,
  merchant_name VARCHAR(128) NOT NULL,
  contact_name VARCHAR(64) NOT NULL,
  contact_phone VARCHAR(20) NOT NULL,
  contact_email VARCHAR(128) NULL,
  license_no VARCHAR(64) NULL,
  license_file_id BIGINT NULL,
  review_status VARCHAR(16) NOT NULL,
  reject_reason VARCHAR(255) NULL,
  reviewed_by BIGINT NULL,
  reviewed_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME NULL,
  INDEX idx_review_status_created (review_status, created_at),
  INDEX idx_contact_phone (contact_phone)
);

CREATE TABLE IF NOT EXISTS merchant_accounts (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  merchant_id BIGINT NOT NULL,
  username VARCHAR(64) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL,
  role VARCHAR(16) NOT NULL,
  status VARCHAR(16) NOT NULL,
  last_login_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME NULL,
  INDEX idx_merchant_role (merchant_id, role)
);

CREATE TABLE IF NOT EXISTS admin_users (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  username VARCHAR(64) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL,
  display_name VARCHAR(64) NOT NULL,
  role VARCHAR(16) NOT NULL,
  status VARCHAR(16) NOT NULL,
  last_login_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME NULL
);

CREATE TABLE IF NOT EXISTS merchant_audit_logs (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  merchant_id BIGINT NOT NULL,
  action VARCHAR(32) NOT NULL,
  from_status VARCHAR(16) NOT NULL,
  to_status VARCHAR(16) NOT NULL,
  reason VARCHAR(255) NULL,
  operator_type VARCHAR(16) NOT NULL,
  operator_id BIGINT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_merchant_created (merchant_id, created_at)
);

CREATE TABLE IF NOT EXISTS categories (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  parent_id BIGINT NULL,
  level TINYINT NOT NULL,
  name VARCHAR(64) NOT NULL,
  status VARCHAR(16) NOT NULL,
  sort INT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME NULL,
  UNIQUE KEY uk_parent_name (parent_id, name),
  INDEX idx_parent_sort (parent_id, sort),
  INDEX idx_level_status_sort (level, status, sort)
);

CREATE TABLE IF NOT EXISTS products (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  product_no VARCHAR(32) NOT NULL UNIQUE,
  merchant_id BIGINT NOT NULL,
  title VARCHAR(128) NOT NULL,
  description TEXT NOT NULL,
  category_id BIGINT NOT NULL,
  price_cent INT NOT NULL,
  original_price_cent INT NULL,
  condition_level VARCHAR(16) NOT NULL,
  stock INT NOT NULL DEFAULT 1,
  cover_file_id BIGINT NULL,
  status VARCHAR(16) NOT NULL,
  active_order_id BIGINT NULL,
  locked_at DATETIME NULL,
  shelf_at DATETIME NULL,
  off_shelf_at DATETIME NULL,
  sold_at DATETIME NULL,
  closed_at DATETIME NULL,
  created_by BIGINT NOT NULL,
  updated_by BIGINT NOT NULL,
  version INT NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME NULL,
  INDEX idx_merchant_status_updated (merchant_id, status, updated_at),
  INDEX idx_merchant_title (merchant_id, title),
  INDEX idx_active_order (active_order_id)
);

CREATE TABLE IF NOT EXISTS product_images (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  product_id BIGINT NOT NULL,
  file_id BIGINT NOT NULL,
  sort_order INT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_product_sort (product_id, sort_order)
);

CREATE TABLE IF NOT EXISTS orders (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  order_no VARCHAR(32) NOT NULL UNIQUE,
  merchant_id BIGINT NOT NULL,
  product_id BIGINT NOT NULL,
  deal_price_cent INT NOT NULL,
  buyer_contact_masked VARCHAR(64) NULL,
  remark VARCHAR(255) NULL,
  status VARCHAR(16) NOT NULL,
  is_active TINYINT(1) NOT NULL,
  close_reason VARCHAR(255) NULL,
  created_by BIGINT NOT NULL,
  completed_at DATETIME NULL,
  closed_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME NULL,
  UNIQUE KEY uk_product_active (product_id, is_active),
  INDEX idx_merchant_status_created (merchant_id, status, created_at),
  INDEX idx_product_id (product_id)
);

CREATE TABLE IF NOT EXISTS order_events (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  order_id BIGINT NOT NULL,
  event_type VARCHAR(32) NOT NULL,
  from_status VARCHAR(16) NULL,
  to_status VARCHAR(16) NOT NULL,
  operator_type VARCHAR(16) NOT NULL,
  operator_id BIGINT NOT NULL,
  note VARCHAR(255) NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_order_created (order_id, created_at)
);

CREATE TABLE IF NOT EXISTS files (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  biz_type VARCHAR(32) NOT NULL,
  object_key VARCHAR(255) NOT NULL UNIQUE,
  url VARCHAR(500) NOT NULL,
  mime_type VARCHAR(64) NOT NULL,
  size_bytes BIGINT NOT NULL,
  uploader_type VARCHAR(16) NOT NULL,
  uploader_id BIGINT NULL,
  scan_status VARCHAR(16) NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_biz_type_created (biz_type, created_at)
);

CREATE TABLE IF NOT EXISTS operation_logs (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  request_id VARCHAR(64) NOT NULL,
  operator_type VARCHAR(16) NOT NULL,
  operator_id BIGINT NOT NULL,
  merchant_id BIGINT NULL,
  action VARCHAR(64) NOT NULL,
  resource_type VARCHAR(32) NOT NULL,
  resource_id BIGINT NOT NULL,
  from_status VARCHAR(16) NULL,
  to_status VARCHAR(16) NULL,
  method VARCHAR(8) NOT NULL,
  path VARCHAR(255) NOT NULL,
  ip VARCHAR(64) NOT NULL,
  user_agent VARCHAR(255) NOT NULL,
  result_code INT NOT NULL,
  detail_json JSON NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_operator_created (operator_type, operator_id, created_at),
  INDEX idx_resource_created (resource_type, resource_id, created_at),
  INDEX idx_merchant_created (merchant_id, created_at)
);

CREATE TABLE IF NOT EXISTS auth_sessions (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  user_type VARCHAR(16) NOT NULL,
  user_id BIGINT NOT NULL,
  refresh_token_hash VARCHAR(255) NOT NULL,
  device_info VARCHAR(255) NULL,
  ip VARCHAR(64) NULL,
  expired_at DATETIME NOT NULL,
  revoked_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_user_expired (user_type, user_id, expired_at),
  INDEX idx_token_hash (refresh_token_hash)
);

CREATE TABLE IF NOT EXISTS idempotency_records (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  idem_key VARCHAR(128) NOT NULL,
  operator_id BIGINT NOT NULL,
  path VARCHAR(255) NOT NULL,
  request_hash VARCHAR(64) NOT NULL,
  result_code INT NOT NULL,
  response_raw JSON NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_idem_scope (idem_key, operator_id, path)
);
