package app

import (
	"errors"
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

func findOrCreateCategory(db *gorm.DB, parentID *uint64, level int8, name string, sort int) (model.Category, error) {
	var category model.Category
	if err := db.Model(&model.Category{}).Where("name = ?", name).First(&category).Error; err != nil {
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

func sameParentID(a, b *uint64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
