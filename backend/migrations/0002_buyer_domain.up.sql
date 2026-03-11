CREATE TABLE IF NOT EXISTS buyer_users (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  buyer_no VARCHAR(32) NOT NULL UNIQUE,
  openid VARCHAR(64) NOT NULL UNIQUE,
  unionid VARCHAR(64) NULL,
  nickname VARCHAR(64) NULL,
  avatar_url VARCHAR(500) NULL,
  phone VARCHAR(20) NULL,
  status VARCHAR(16) NOT NULL,
  last_login_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME NULL,
  INDEX idx_unionid (unionid),
  INDEX idx_status_created (status, created_at)
);

CREATE TABLE IF NOT EXISTS buyer_device_bindings (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  device_id VARCHAR(64) NOT NULL,
  buyer_id BIGINT NOT NULL,
  first_bind_at DATETIME NOT NULL,
  last_bind_at DATETIME NOT NULL,
  last_merge_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_device_buyer (device_id, buyer_id),
  INDEX idx_buyer_last_bind (buyer_id, last_bind_at),
  INDEX idx_device_last_bind (device_id, last_bind_at)
);

CREATE TABLE IF NOT EXISTS buyer_favorites (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  owner_type VARCHAR(16) NOT NULL,
  owner_key VARCHAR(96) NOT NULL,
  buyer_id BIGINT NULL,
  device_id VARCHAR(64) NULL,
  product_id BIGINT NOT NULL,
  merchant_id BIGINT NOT NULL,
  is_active TINYINT(1) NOT NULL DEFAULT 1,
  merge_target_buyer_id BIGINT NULL,
  merged_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_buyer_favorite_owner_product (owner_key, product_id),
  INDEX idx_buyer_active_created (buyer_id, is_active, created_at),
  INDEX idx_device_active_created (device_id, is_active, created_at),
  INDEX idx_product_active_created (product_id, is_active, created_at)
);

CREATE TABLE IF NOT EXISTS buyer_histories (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  owner_type VARCHAR(16) NOT NULL,
  owner_key VARCHAR(96) NOT NULL,
  buyer_id BIGINT NULL,
  device_id VARCHAR(64) NULL,
  product_id BIGINT NOT NULL,
  merchant_id BIGINT NOT NULL,
  first_viewed_at DATETIME NOT NULL,
  last_viewed_at DATETIME NOT NULL,
  view_count INT NOT NULL,
  is_active TINYINT(1) NOT NULL DEFAULT 1,
  merge_target_buyer_id BIGINT NULL,
  merged_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_buyer_history_owner_product (owner_key, product_id),
  INDEX idx_owner_last_view (owner_key, is_active, last_viewed_at),
  INDEX idx_buyer_last_view (buyer_id, is_active, last_viewed_at),
  INDEX idx_device_last_view (device_id, is_active, last_viewed_at),
  INDEX idx_product_last_view (product_id, last_viewed_at)
);

CREATE TABLE IF NOT EXISTS buyer_intents (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  intent_no VARCHAR(32) NOT NULL UNIQUE,
  buyer_id BIGINT NOT NULL,
  source_device_id VARCHAR(64) NULL,
  product_id BIGINT NOT NULL,
  merchant_id BIGINT NOT NULL,
  status VARCHAR(16) NOT NULL,
  is_open TINYINT(1) NOT NULL DEFAULT 1,
  contact_name VARCHAR(64) NULL,
  contact_phone VARCHAR(20) NULL,
  contact_wechat VARCHAR(64) NULL,
  message VARCHAR(500) NULL,
  handled_by BIGINT NULL,
  handled_at DATETIME NULL,
  closed_at DATETIME NULL,
  close_reason VARCHAR(32) NULL,
  merchant_note VARCHAR(255) NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_buyer_product_open (buyer_id, product_id, is_open),
  INDEX idx_buyer_intent_merchant_status_created (merchant_id, status, created_at),
  INDEX idx_buyer_intent_buyer_created (buyer_id, created_at),
  INDEX idx_buyer_intent_product_open (product_id, is_open),
  INDEX idx_buyer_intent_source_device_created (source_device_id, created_at)
);
