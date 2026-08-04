CREATE TABLE IF NOT EXISTS image_backfill_runs (
  id VARCHAR(64) NOT NULL,
  profile_version VARCHAR(32) NOT NULL,
  created_at DATETIME(3) NULL,
  finished_at DATETIME(3) NULL,
  PRIMARY KEY (id),
  KEY idx_image_backfill_runs_profile_version (profile_version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS image_backfill_items (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  run_id VARCHAR(64) NOT NULL,
  file_id BIGINT UNSIGNED NOT NULL,
  source_object_key VARCHAR(255) NOT NULL,
  target_object_key VARCHAR(255) NOT NULL,
  profile_version VARCHAR(32) NOT NULL,
  source_sha256 VARCHAR(64) NULL,
  output_sha256 VARCHAR(64) NULL,
  source_size_bytes BIGINT NOT NULL DEFAULT 0,
  output_size_bytes BIGINT NULL,
  status VARCHAR(16) NOT NULL,
  attempts INT NOT NULL DEFAULT 0,
  error_code VARCHAR(64) NULL,
  committed_at DATETIME(3) NULL,
  cleanup_after DATETIME(3) NULL,
  cleanup_status VARCHAR(16) NOT NULL DEFAULT 'NOT_SCHEDULED',
  cleanup_error_code VARCHAR(64) NULL,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_backfill_run_file (run_id, file_id),
  KEY idx_image_backfill_items_file_id (file_id),
  KEY idx_image_backfill_items_profile_version (profile_version),
  KEY idx_image_backfill_items_status (status),
  KEY idx_image_backfill_items_cleanup_status (cleanup_status),
  KEY idx_image_backfill_items_cleanup_after (cleanup_after)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
