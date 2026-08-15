# Merchant Category Management Design

## Goal

Add merchant-owned category management for first-level and second-level product categories. Each merchant can manage only its own categories in the shared backend/admin application. Each merchant miniapp entry shows only that merchant's categories and products.

This design includes a lightweight default initialization path: copy the existing built-in default category tree to each merchant. It does not include a platform category-template management UI or multiple configurable templates.

## Current State

- `categories` is a global two-level dictionary without `merchant_id`.
- `products.category_id` points to a global level-2 category.
- Merchant backend exposes `GET /api/v1/merchant/categories`, but it is read-only and filters the seeded defaults.
- Buyer miniapp exposes `GET /api/v1/buyer/categories` and `GET /api/v1/buyer/products`, both currently read global or cross-merchant data.
- Frontend merchant navigation has no category-management menu.
- Miniapp builds already contain merchant-specific store constants, but API requests do not consistently carry a merchant identity.

## Scope

In scope:

- Add merchant ownership to categories.
- Add merchant category CRUD APIs for two-level categories.
- Add a merchant backend menu and page for managing categories.
- Seed every existing and new merchant with a copy of the built-in default category tree.
- Remap existing products from legacy global categories to the owning merchant's copied category.
- Require product creation and category edits to use an enabled level-2 category owned by the current merchant.
- Make buyer miniapp category, product, favorite, history, and intent surfaces respect the current merchant entry.
- Add backend, frontend, and miniapp regression coverage for the behavior above.

Out of scope:

- Platform/admin template management.
- Multiple category templates.
- Marketplace-wide category merging.
- Cross-merchant product discovery inside one merchant miniapp.
- Bulk import/export of categories.

## Data Model

`categories` gains `merchant_id`.

- New and active application-owned categories must have `merchant_id`.
- Existing legacy global rows may remain with `merchant_id IS NULL` after migration, but runtime merchant and buyer queries must ignore them.
- Parent-child relationships must stay inside one merchant.
- Level 1 categories have `parent_id = NULL`.
- Level 2 categories have `parent_id` pointing to a level 1 category owned by the same merchant.

Uniqueness is enforced for non-deleted categories by service validation and supporting indexes:

- A merchant cannot have duplicate non-deleted level-1 category names.
- A merchant cannot have duplicate non-deleted child names under the same parent.
- Different merchants may use the same category names.

The Go `model.Category` struct will expose `merchant_id` in API JSON responses.

## Migration And Initialization

Add a new migration after the current latest migration.

Migration behavior:

- Add `merchant_id` to `categories`.
- Add indexes for merchant/category lookup and parent ordering.
- For each merchant, create a private copy of the built-in `defaultCategorySeeds` tree.
- Build a mapping from legacy global level-2 categories to each merchant's copied level-2 categories by root name and child name.
- Update every product to point to the copied level-2 category for its `merchant_id`.
- Leave legacy global category rows in place with `merchant_id IS NULL` so the migration is less destructive and reversible by backup/rollback procedures.

Runtime initialization behavior:

- New merchant registration creates the merchant's default category tree in the same transaction after merchant/account creation.
- The initializer is idempotent by merchant and category name, so repeated calls repair missing rows without duplicating categories.
- The initializer uses the same built-in seed list as the migration.

## Backend API

Merchant APIs:

- `GET /api/v1/merchant/categories`
  - Returns only categories where `merchant_id = actor.MerchantID`.
  - Supports existing `level`, `parent_id`, and status filtering.
- `POST /api/v1/merchant/categories`
  - Creates a level-1 or level-2 category for the current merchant.
  - Requires `parent_id` for level 2.
  - Rejects cross-merchant parents and duplicate names.
- `PUT /api/v1/merchant/categories/:id`
  - Updates name, sort, and status for a category owned by the current merchant.
  - Rejects moving between merchants or changing category level.
- `DELETE /api/v1/merchant/categories/:id`
  - Soft-deletes a category owned by the current merchant.
  - Rejects deletion when products reference the category.
  - Rejects deletion of a level-1 category while it still has children.

Product behavior:

- Product create/update validates `category_id` with `merchant_id = actor.MerchantID`, `level = 2`, and `status = ENABLED`.
- Product list/detail joins continue to resolve category names from the category ID. Legacy global categories should not be assigned after migration.

Buyer APIs:

- Buyer requests must include the current merchant entry identity as `merchant_no`.
- The backend resolves `merchant_no` to `merchants.id` and uses it as the buyer merchant scope.
- Buyer categories and product listing return only that merchant's enabled categories and visible products.
- Product detail validates that the product belongs to the scoped merchant before returning it.
- Favorite, history, and intent list endpoints filter to the scoped merchant.
- Favorite, history, and intent write endpoints validate that the target product belongs to the scoped merchant.

## Frontend

Merchant admin app:

- Add a merchant menu item named `商品分类`.
- Add route `/merchant/categories`.
- Add a category management page that shows level-1 categories and their level-2 children.
- Support create, rename, sort edit, enable/disable, and delete.
- Disable destructive actions while a request is pending.
- Surface backend validation errors as normal form or message errors.
- Keep styling aligned with the existing Ant Design Pro merchant pages.

Product create/edit forms:

- Continue using `GET /merchant/categories`, now scoped to the merchant.
- Only selectable level-2 enabled categories are submitted as `category_id`.

Miniapp:

- Add a merchant entry configuration named `TARO_APP_MERCHANT_NO`.
- Attach the merchant identity to buyer API requests as `merchant_no` query data.
- Category pages and product lists use only categories/products for the configured merchant.
- Product detail, favorite, history, and intent operations include the same merchant identity.

## Error Handling

- Missing `merchant_no` on buyer scoped endpoints returns invalid-argument using the existing API error envelope.
- Unknown `merchant_no` on buyer scoped endpoints returns not-found using the existing API error envelope.
- Cross-merchant category or product access returns not-found to avoid exposing another merchant's resources.
- Deleting a category with child categories or product references returns invalid-argument.
- Creating duplicate category names returns invalid-argument.
- Migration should fail atomically if a product category cannot be mapped to the merchant-owned copy.

## Testing

Backend tests:

- Migration/backfill creates per-merchant default categories and remaps products.
- Category CRUD enforces merchant ownership, duplicate-name rules, level constraints, and delete blockers.
- Product create/update rejects categories from other merchants and disabled categories.
- Buyer category/product/detail/favorite/history/intent endpoints respect `merchant_no`.
- Initializer is idempotent for repeated calls.

Frontend tests:

- Merchant layout includes the category menu for merchant users.
- Category page can render, create, edit, disable, and delete with mocked API responses.
- Product forms still load and submit level-2 categories.

Miniapp tests:

- Buyer API service includes the configured merchant identity.
- Category and product fetches pass the merchant scope.
- Product-related writes carry the same merchant scope.

## Rollout Notes

- Deploy backend migration before relying on merchant category CRUD.
- Existing legacy global categories are intentionally left unused rather than deleted.
- Existing products must be checked after migration to confirm all `category_id` values point to categories with the same `merchant_id`.
- Real credentials and merchant-specific miniapp build secrets must stay out of the repository.
