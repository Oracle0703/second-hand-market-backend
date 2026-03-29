ALTER TABLE buyer_users
  DROP INDEX uk_buyer_provider_openid,
  DROP COLUMN auth_provider,
  ADD UNIQUE KEY openid (openid);
