ALTER TABLE buyer_users
  ADD COLUMN auth_provider VARCHAR(16) NOT NULL DEFAULT 'wechat' AFTER buyer_no;

ALTER TABLE buyer_users
  DROP INDEX openid,
  ADD UNIQUE KEY uk_buyer_provider_openid (auth_provider, openid);
