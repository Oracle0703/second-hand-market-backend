package app

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/dto"
	"second-hand-market-backend/backend/internal/model"
)

func (s *Server) handleCreateCategory(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	var req dto.CreateCategoryRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	category, err := s.createOwnedCategory(s.DB, actor.MerchantID, req)
	if err != nil {
		common.Fail(c, err)
		return
	}
	common.Success(c, gin.H{
		"id":          category.ID,
		"merchant_id": category.MerchantID,
		"parent_id":   category.ParentID,
		"level":       category.Level,
		"name":        category.Name,
		"status":      category.Status,
		"sort":        category.Sort,
		"created_at":  category.CreatedAt,
		"updated_at":  category.UpdatedAt,
	})
}

func (s *Server) handleUpdateCategory(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	var req dto.UpdateCategoryRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	category, err := s.updateOwnedCategory(s.DB, actor.MerchantID, id, req)
	if err != nil {
		common.Fail(c, err)
		return
	}
	common.Success(c, gin.H{"item": category})
}

func (s *Server) handleDeleteCategory(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	if err := s.deleteOwnedCategory(s.DB, actor.MerchantID, id); err != nil {
		common.Fail(c, err)
		return
	}
	common.Success(c, gin.H{"success": true})
}

func (s *Server) createOwnedCategory(db *gorm.DB, merchantID uint64, req dto.CreateCategoryRequest) (model.Category, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return model.Category{}, common.ErrInvalidArgument
	}
	sort := req.Sort
	if sort < 0 {
		return model.Category{}, common.ErrInvalidArgument
	}

	var parentID *uint64
	switch req.Level {
	case 1:
		if req.ParentID != nil {
			return model.Category{}, common.ErrInvalidArgument
		}
	case 2:
		if req.ParentID == nil {
			return model.Category{}, common.ErrInvalidArgument
		}
		parent, err := s.loadOwnedCategory(db, *req.ParentID, merchantID)
		if err != nil || parent.Level != 1 {
			return model.Category{}, common.ErrInvalidArgument
		}
		parentID = req.ParentID
	default:
		return model.Category{}, common.ErrInvalidArgument
	}

	var existing model.Category
	query := db.Unscoped().Where("merchant_id = ? AND level = ? AND name = ?", merchantID, req.Level, name)
	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}
	err := query.First(&existing).Error
	if err == nil {
		if !existing.DeletedAt.Valid {
			return model.Category{}, common.ErrInvalidArgument
		}
		updates := map[string]interface{}{
			"deleted_at": nil,
			"status":     model.CategoryEnabled,
			"sort":       sort,
		}
		if err := db.Unscoped().Model(&model.Category{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
			return model.Category{}, err
		}
		existing.DeletedAt.Valid = false
		existing.Status = model.CategoryEnabled
		existing.Sort = sort
		return existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Category{}, err
	}

	merchantIDValue := merchantID
	category := model.Category{
		MerchantID: &merchantIDValue,
		ParentID:   parentID,
		Level:      req.Level,
		Name:       name,
		Status:     model.CategoryEnabled,
		Sort:       sort,
	}
	if err := db.Create(&category).Error; err != nil {
		return model.Category{}, err
	}
	return category, nil
}

func (s *Server) updateOwnedCategory(db *gorm.DB, merchantID, categoryID uint64, req dto.UpdateCategoryRequest) (model.Category, error) {
	category, err := s.loadOwnedCategory(db, categoryID, merchantID)
	if err != nil {
		return model.Category{}, err
	}
	updates := map[string]interface{}{}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return model.Category{}, common.ErrInvalidArgument
		}
		if name != category.Name {
			conflict, err := categoryNameConflict(db, merchantID, category.ParentID, category.Level, name, category.ID)
			if err != nil {
				return model.Category{}, err
			}
			if conflict {
				return model.Category{}, common.ErrInvalidArgument
			}
			updates["name"] = name
			category.Name = name
		}
	}
	if req.Sort != nil {
		if *req.Sort < 0 {
			return model.Category{}, common.ErrInvalidArgument
		}
		updates["sort"] = *req.Sort
		category.Sort = *req.Sort
	}
	if req.Status != nil {
		status := strings.TrimSpace(*req.Status)
		if status != model.CategoryEnabled && status != model.CategoryDisabled {
			return model.Category{}, common.ErrInvalidArgument
		}
		updates["status"] = status
		category.Status = status
	}
	if len(updates) == 0 {
		return category, nil
	}
	if err := db.Model(&model.Category{}).Where("id = ?", category.ID).Updates(updates).Error; err != nil {
		return model.Category{}, err
	}
	return category, nil
}

func (s *Server) deleteOwnedCategory(db *gorm.DB, merchantID, categoryID uint64) error {
	return db.Transaction(func(tx *gorm.DB) error {
		category, err := s.loadOwnedCategory(tx, categoryID, merchantID)
		if err != nil {
			return err
		}

		if category.Level == 1 {
			var children []model.Category
			if err := tx.Where("merchant_id = ? AND parent_id = ? AND level = ?", merchantID, category.ID, 2).
				Order("id ASC").Find(&children).Error; err != nil {
				return err
			}
			for _, child := range children {
				if err := s.ensureCategoryHasNoProducts(tx, merchantID, child.ID); err != nil {
					return err
				}
			}
			if len(children) > 0 {
				if err := tx.Delete(&children).Error; err != nil {
					return err
				}
			}
		} else if category.Level == 2 {
			if err := s.ensureCategoryHasNoProducts(tx, merchantID, category.ID); err != nil {
				return err
			}
		} else {
			return common.ErrInvalidArgument
		}

		return tx.Delete(&category).Error
	})
}

func (s *Server) ensureCategoryHasNoProducts(db *gorm.DB, merchantID, categoryID uint64) error {
	var productCount int64
	if err := db.Model(&model.Product{}).Where("merchant_id = ? AND category_id = ?", merchantID, categoryID).Count(&productCount).Error; err != nil {
		return err
	}
	if productCount > 0 {
		return common.ErrInvalidArgument
	}
	return nil
}

func (s *Server) loadOwnedCategory(db *gorm.DB, categoryID uint64, merchantID uint64) (model.Category, error) {
	target := s.DB
	if db != nil {
		target = db
	}
	var category model.Category
	if err := target.Where("id = ? AND merchant_id = ?", categoryID, merchantID).First(&category).Error; err != nil {
		return model.Category{}, s.dbError(err)
	}
	return category, nil
}

func (s *Server) ensureMerchantLevel2Category(db *gorm.DB, merchantID uint64, categoryID uint64) error {
	target := s.DB
	if db != nil {
		target = db
	}
	var category model.Category
	if err := target.Where("id = ? AND merchant_id = ?", categoryID, merchantID).First(&category).Error; err != nil {
		return common.ErrInvalidArgument
	}
	if category.Level != 2 || category.Status != model.CategoryEnabled {
		return common.ErrInvalidArgument
	}
	return nil
}

func categoryNameConflict(db *gorm.DB, merchantID uint64, parentID *uint64, level int8, name string, excludeID uint64) (bool, error) {
	query := db.Model(&model.Category{}).Where("merchant_id = ? AND level = ? AND name = ? AND id <> ?", merchantID, level, name, excludeID)
	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
