package app

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/model"
)

type AdminBootstrap struct {
	Username    string
	DisplayName string
	Role        string
	Password    string
}

func MigrateSchema(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.Merchant{},
		&model.MerchantAccount{},
		&model.AdminUser{},
		&model.MerchantAuditLog{},
		&model.Category{},
		&model.Product{},
		&model.ProductImage{},
		&model.ProductStockAdjustment{},
		&model.Order{},
		&model.OrderEvent{},
		&model.FileRecord{},
		&model.ImageBackfillRun{},
		&model.ImageBackfillItem{},
		&model.OperationLog{},
		&model.AuthSession{},
		&model.IdempotencyRecord{},
		&model.BuyerUser{},
		&model.BuyerDeviceBinding{},
		&model.BuyerFavorite{},
		&model.BuyerHistory{},
		&model.BuyerIntent{},
	)
}

func BootstrapAdmin(db *gorm.DB, bootstrap AdminBootstrap) error {
	username := strings.TrimSpace(bootstrap.Username)
	if username == "" {
		return errors.New("ADMIN_USERNAME is required")
	}
	if bootstrap.Password == "" {
		return errors.New("ADMIN_PASSWORD is required")
	}
	if bootstrap.Role != model.AdminRoleAdmin && bootstrap.Role != model.AdminRoleSuper {
		return errors.New("ADMIN_ROLE is invalid")
	}

	var existing model.AdminUser
	err := db.Where("username = ?", username).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New("ADMIN_BOOTSTRAP query failed")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(bootstrap.Password), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("ADMIN_PASSWORD is invalid")
	}
	admin := model.AdminUser{
		Username:     username,
		DisplayName:  strings.TrimSpace(bootstrap.DisplayName),
		Role:         bootstrap.Role,
		Status:       model.AccountStatusActive,
		PasswordHash: string(hash),
	}
	if err := db.Create(&admin).Error; err != nil {
		return errors.New("ADMIN_BOOTSTRAP create failed")
	}
	return nil
}

type categorySeed struct {
	Name     string
	Children []string
}

var defaultCategorySeeds = []categorySeed{
	{Name: "家具类", Children: []string{"家具", "家电", "麻将机", "商铺用品"}},
	{Name: "办公类", Children: []string{"老板桌", "办公桌", "老板椅", "老板办公座椅套装", "会议桌", "办公沙发", "会议桌椅套装", "文件柜书柜"}},
	{Name: "麻将机类", Children: []string{"旧麻将机", "新麻将机", "麻将椅", "茶几", "麻将机维修"}},
}

func SeedDefaultCategories(db *gorm.DB) error {
	for i, seed := range defaultCategorySeeds {
		root, err := findOrCreateCategory(db, nil, 1, seed.Name, i+1)
		if err != nil {
			return err
		}
		seen := map[string]struct{}{}
		sortOrder := 1
		for _, childName := range seed.Children {
			name := strings.TrimSpace(childName)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			if _, err := findOrCreateCategory(db, &root.ID, 2, name, sortOrder); err != nil {
				return err
			}
			sortOrder++
		}
	}
	return nil
}

func EnsureMerchantDefaultCategories(db *gorm.DB, merchantID uint64) error {
	if merchantID == 0 {
		return errors.New("merchant_id is required")
	}
	for i, seed := range defaultCategorySeeds {
		root, err := findOrCreateMerchantCategory(db, merchantID, nil, 1, seed.Name, i+1)
		if err != nil {
			return err
		}
		seen := map[string]struct{}{}
		sortOrder := 1
		for _, childName := range seed.Children {
			name := strings.TrimSpace(childName)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			if _, err := findOrCreateMerchantCategory(db, merchantID, &root.ID, 2, name, sortOrder); err != nil {
				return err
			}
			sortOrder++
		}
	}
	return nil
}

func BackfillMerchantCategories(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var merchants []model.Merchant
		if err := tx.Order("id ASC").Find(&merchants).Error; err != nil {
			return err
		}
		for _, merchant := range merchants {
			if err := EnsureMerchantDefaultCategories(tx, merchant.ID); err != nil {
				return err
			}
			if err := remapMerchantProductsToOwnedCategories(tx, merchant.ID); err != nil {
				return err
			}
		}
		return nil
	})
}

func remapMerchantProductsToOwnedCategories(db *gorm.DB, merchantID uint64) error {
	var products []model.Product
	if err := db.Where("merchant_id = ?", merchantID).Order("id ASC").Find(&products).Error; err != nil {
		return err
	}
	for _, product := range products {
		var current model.Category
		err := db.Where("id = ?", product.CategoryID).First(&current).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("product %d category %d not found", product.ID, product.CategoryID)
		}
		if err != nil {
			return err
		}
		if current.MerchantID != nil {
			if *current.MerchantID != merchantID {
				return fmt.Errorf("product %d category %d belongs to merchant %d", product.ID, current.ID, *current.MerchantID)
			}
			continue
		}
		if current.Level != 2 || current.ParentID == nil {
			return fmt.Errorf("product %d category %d is not a legacy level-2 category", product.ID, current.ID)
		}
		var legacyRoot model.Category
		if err := db.Where("id = ? AND merchant_id IS NULL AND level = ?", *current.ParentID, 1).First(&legacyRoot).Error; err != nil {
			return err
		}
		var ownedRoot model.Category
		if err := db.Where("merchant_id = ? AND parent_id IS NULL AND level = ? AND name = ?", merchantID, 1, legacyRoot.Name).First(&ownedRoot).Error; err != nil {
			return err
		}
		var ownedChild model.Category
		if err := db.Where("merchant_id = ? AND parent_id = ? AND level = ? AND name = ?", merchantID, ownedRoot.ID, 2, current.Name).First(&ownedChild).Error; err != nil {
			return err
		}
		if err := db.Model(&model.Product{}).Where("id = ?", product.ID).Update("category_id", ownedChild.ID).Error; err != nil {
			return err
		}
	}
	return nil
}

func findOrCreateCategory(db *gorm.DB, parentID *uint64, level int8, name string, sort int) (model.Category, error) {
	var category model.Category
	query := db.Model(&model.Category{}).Where("merchant_id IS NULL AND level = ? AND name = ?", level, name)
	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}
	if err := query.First(&category).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			category = model.Category{
				ParentID: parentID,
				Level:    level,
				Name:     name,
				Status:   model.CategoryEnabled,
				Sort:     sort,
			}
			return category, db.Create(&category).Error
		}
		return model.Category{}, err
	}
	updates := map[string]interface{}{}
	if !sameParentID(category.ParentID, parentID) {
		updates["parent_id"] = parentID
	}
	if category.Level != level {
		updates["level"] = level
	}
	if category.Status != model.CategoryEnabled {
		updates["status"] = model.CategoryEnabled
	}
	if category.Sort != sort {
		updates["sort"] = sort
	}
	if len(updates) > 0 {
		if err := db.Model(&model.Category{}).Where("id = ?", category.ID).Updates(updates).Error; err != nil {
			return model.Category{}, err
		}
		category.Status = model.CategoryEnabled
		category.Sort = sort
	}
	return category, nil
}

func findOrCreateMerchantCategory(db *gorm.DB, merchantID uint64, parentID *uint64, level int8, name string, sort int) (model.Category, error) {
	name = strings.TrimSpace(name)
	var category model.Category
	query := db.Unscoped().Model(&model.Category{}).Where("merchant_id = ? AND level = ? AND name = ?", merchantID, level, name)
	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}
	if err := query.First(&category).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			merchantIDValue := merchantID
			category = model.Category{
				MerchantID: &merchantIDValue,
				ParentID:   parentID,
				Level:      level,
				Name:       name,
				Status:     model.CategoryEnabled,
				Sort:       sort,
			}
			return category, db.Create(&category).Error
		}
		return model.Category{}, err
	}
	if category.DeletedAt.Valid || category.Status != model.CategoryEnabled || category.Sort != sort {
		updates := map[string]interface{}{
			"deleted_at": nil,
			"status":     model.CategoryEnabled,
			"sort":       sort,
		}
		if err := db.Unscoped().Model(&model.Category{}).Where("id = ?", category.ID).Updates(updates).Error; err != nil {
			return model.Category{}, err
		}
		category.DeletedAt.Valid = false
		category.Status = model.CategoryEnabled
		category.Sort = sort
	}
	return category, nil
}

func sameParentID(a, b *uint64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
