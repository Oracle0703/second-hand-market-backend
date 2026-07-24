-- Stable, non-PII fingerprints for the protected production-derived account.
-- Excludes migration-owned fields such as reserved_stock and active_order_id.

SELECT
  'yaner_account' AS protected_scope,
  COUNT(*) AS row_count,
  SHA2(
    COALESCE(
      GROUP_CONCAT(
        CONCAT_WS('#', id, merchant_id, username, role, status, password_hash)
        ORDER BY id SEPARATOR '|'
      ),
      ''
    ),
    256
  ) AS stable_fingerprint
FROM merchant_accounts
WHERE username = 'yaner' AND deleted_at IS NULL;

SELECT
  'yaner_products' AS protected_scope,
  COUNT(*) AS row_count,
  SHA2(
    COALESCE(
      GROUP_CONCAT(
        CONCAT_WS(
          '#',
          p.id,
          p.product_no,
          p.status,
          p.stock,
          p.price_cent,
          COALESCE(CAST(p.original_price_cent AS CHAR), 'NULL')
        )
        ORDER BY p.id SEPARATOR '|'
      ),
      ''
    ),
    256
  ) AS stable_fingerprint
FROM products AS p
INNER JOIN merchant_accounts AS account
  ON account.merchant_id = p.merchant_id
WHERE account.username = 'yaner'
  AND account.deleted_at IS NULL
  AND p.deleted_at IS NULL;
