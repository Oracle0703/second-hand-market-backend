package main

import (
	"database/sql"
	"errors"
	"log"
	"os"
	"strings"
	"sync"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"second-hand-market-backend/backend/internal/databasecmd"
	"second-hand-market-backend/backend/internal/model"
)

const defaultCategorySeedID = "default-categories"
const categorySeedLockName = "second-hand-market.default-categories.v1"

var categorySeedProcessLock sync.Mutex

type categorySeedDependencies struct {
	loadConfig    func() (databasecmd.Config, error)
	openDatabase  func(databasecmd.Config) (*gorm.DB, error)
	closeDatabase func(*gorm.DB)
	seed          func(*gorm.DB) error
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

func main() {
	seedID, err := run(os.Args[1:], defaultCategorySeedDependencies())
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	log.Printf("CATEGORY_SEED PASS seed=%s", seedID)
}

func defaultCategorySeedDependencies() categorySeedDependencies {
	return categorySeedDependencies{
		loadConfig:    databasecmd.LoadConfig,
		openDatabase:  databasecmd.OpenDatabase,
		closeDatabase: databasecmd.CloseDatabase,
		seed:          seedDefaultCategories,
	}
}

func run(args []string, dependencies categorySeedDependencies) (string, error) {
	seedID, err := parseCategorySeedSelection(args)
	if err != nil {
		return "", err
	}

	databaseConfig, err := dependencies.loadConfig()
	if err != nil {
		return "", err
	}
	db, err := dependencies.openDatabase(databaseConfig)
	if err != nil {
		return "", err
	}
	defer dependencies.closeDatabase(db)

	if err := dependencies.seed(db); err != nil {
		return "", errors.New("CATEGORY_SEED failed seed=" + seedID)
	}
	return seedID, nil
}

func parseCategorySeedSelection(args []string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("CATEGORY_SEED_SELECTION is required")
	}

	var selection string
	switch {
	case len(args) == 1 && strings.HasPrefix(args[0], "--seed="):
		selection = strings.TrimPrefix(args[0], "--seed=")
	case len(args) == 2 && args[0] == "--seed":
		selection = args[1]
	default:
		return "", errors.New("CATEGORY_SEED_ARGUMENTS are invalid")
	}
	if selection == "" {
		return "", errors.New("CATEGORY_SEED_SELECTION is required")
	}
	if selection != defaultCategorySeedID {
		return "", errors.New("CATEGORY_SEED_SELECTION is invalid")
	}
	return selection, nil
}

func seedDefaultCategories(db *gorm.DB) error {
	categorySeedProcessLock.Lock()
	defer categorySeedProcessLock.Unlock()

	if db.Dialector.Name() == "mysql" {
		return db.Connection(func(connection *gorm.DB) error {
			return seedDefaultCategoriesWithMySQLLock(connection)
		})
	}
	return seedDefaultCategoriesTransaction(db)
}

func seedDefaultCategoriesWithMySQLLock(db *gorm.DB) (resultErr error) {
	var acquired sql.NullInt64
	if err := db.Raw("SELECT GET_LOCK(?, 10)", categorySeedLockName).Scan(&acquired).Error; err != nil {
		return errors.New("CATEGORY_SEED lock failed")
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		return errors.New("CATEGORY_SEED lock unavailable")
	}
	defer func() {
		var released sql.NullInt64
		releaseErr := db.Raw("SELECT RELEASE_LOCK(?)", categorySeedLockName).Scan(&released).Error
		if resultErr == nil && (releaseErr != nil || !released.Valid || released.Int64 != 1) {
			resultErr = errors.New("CATEGORY_SEED unlock failed")
		}
	}()

	resultErr = seedDefaultCategoriesTransaction(db)
	return resultErr
}

func seedDefaultCategoriesTransaction(db *gorm.DB) error {
	return db.Session(&gorm.Session{NewDB: true}).Transaction(func(transaction *gorm.DB) error {
		seenRoots := make(map[string]struct{}, len(defaultCategorySeeds))
		for rootIndex, seed := range defaultCategorySeeds {
			rootName := strings.TrimSpace(seed.Name)
			if rootName == "" || rootName != seed.Name {
				return errors.New("CATEGORY_SEED definition is invalid")
			}
			if _, exists := seenRoots[rootName]; exists {
				return errors.New("CATEGORY_SEED definition is invalid")
			}
			seenRoots[rootName] = struct{}{}

			root, err := ensureSeedCategory(transaction, nil, 1, rootName, rootIndex+1)
			if err != nil {
				return err
			}
			if root.ID == 0 {
				return errors.New("CATEGORY_SEED identity is invalid")
			}

			seenChildren := make(map[string]struct{}, len(seed.Children))
			for childIndex, rawName := range seed.Children {
				childName := strings.TrimSpace(rawName)
				if childName == "" || childName != rawName {
					return errors.New("CATEGORY_SEED definition is invalid")
				}
				if _, exists := seenChildren[childName]; exists {
					return errors.New("CATEGORY_SEED definition is invalid")
				}
				seenChildren[childName] = struct{}{}
				if _, err := ensureSeedCategory(transaction, &root.ID, 2, childName, childIndex+1); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func ensureSeedCategory(
	db *gorm.DB,
	parentID *uint64,
	level int8,
	name string,
	sortOrder int,
) (model.Category, error) {
	var matches []model.Category
	query := categoryBusinessKeyQuery(db, parentID, name).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Limit(2)
	if err := query.Find(&matches).Error; err != nil {
		return model.Category{}, errors.New("CATEGORY_SEED query failed")
	}
	if len(matches) > 1 {
		return model.Category{}, errors.New("CATEGORY_SEED identity conflict")
	}
	if len(matches) == 0 {
		category := model.Category{
			ParentID: parentID,
			Level:    level,
			Name:     name,
			Status:   model.CategoryEnabled,
			Sort:     sortOrder,
		}
		if err := db.Create(&category).Error; err != nil {
			return model.Category{}, errors.New("CATEGORY_SEED create failed")
		}
		return category, nil
	}

	category := matches[0]
	if category.ID == 0 ||
		category.DeletedAt.Valid ||
		category.Level != level ||
		category.Name != name ||
		!sameCategoryParentID(category.ParentID, parentID) {
		return model.Category{}, errors.New("CATEGORY_SEED identity conflict")
	}

	updates := categoryMutableUpdates(category, sortOrder)
	if len(updates) == 0 {
		return category, nil
	}

	result := categoryBusinessKeyQuery(db, parentID, name).
		Where("id = ?", category.ID).
		UpdateColumns(updates)
	if result.Error != nil || result.RowsAffected != 1 {
		return model.Category{}, errors.New("CATEGORY_SEED update failed")
	}
	category.Status = model.CategoryEnabled
	category.Sort = sortOrder
	return category, nil
}

func categoryMutableUpdates(category model.Category, sortOrder int) map[string]interface{} {
	updates := map[string]interface{}{}
	if category.Status != model.CategoryEnabled {
		updates["status"] = model.CategoryEnabled
	}
	if category.Sort != sortOrder {
		updates["sort"] = sortOrder
	}
	if len(updates) > 0 {
		updates["updated_at"] = gorm.Expr("updated_at")
	}
	return updates
}

func categoryBusinessKeyQuery(db *gorm.DB, parentID *uint64, name string) *gorm.DB {
	query := db.Unscoped().Model(&model.Category{})
	if parentID == nil {
		return query.Where("parent_id IS NULL AND name = ?", name)
	}
	return query.Where("parent_id = ? AND name = ?", *parentID, name)
}

func sameCategoryParentID(actual, expected *uint64) bool {
	if actual == nil && expected == nil {
		return true
	}
	if actual == nil || expected == nil {
		return false
	}
	return *actual == *expected
}
