package model

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestMultiStockModelMetadataMatchesMigrationContract(t *testing.T) {
	orderSchema, err := schema.Parse(&Order{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse order schema: %v", err)
	}
	productSchema, err := schema.Parse(&Product{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse product schema: %v", err)
	}

	assertFieldTagContains(t, orderSchema, "Quantity",
		"type:int",
		"not null",
		"default:1",
		"check:chk_orders_quantity_positive,quantity > 0",
	)
	assertFieldTagContains(t, orderSchema, "ProductID", "type:bigint", "not null")
	assertFieldTagContains(t, orderSchema, "Status", "size:16", "not null")
	assertFieldTagContains(t, orderSchema, "IsActive", "not null")
	assertFieldTagContains(t, productSchema, "ReservedStock",
		"type:int",
		"not null",
		"default:0",
		"check:chk_products_stock_reservation_bounds,stock >= 0 AND reserved_stock >= 0 AND reserved_stock <= stock",
	)
	assertFieldTagContains(t, productSchema, "Stock",
		"type:int",
		"not null",
		"default:1",
	)
	assertFieldTagContains(t, productSchema, "Status", "size:16", "not null")
	assertFieldTagContains(t, productSchema, "ActiveOrderID", "type:bigint")

	indexes := orderSchema.ParseIndexes()
	assertModelIndex(t, indexes, "idx_product_id", false, "ProductID")
	assertModelIndex(t, indexes, "idx_product_active", false, "ProductID", "IsActive")
	for _, index := range indexes {
		if index.Name == "uk_product_active" {
			t.Fatal("order model still declares the invalid uk_product_active index")
		}
	}

	productIndexes := productSchema.ParseIndexes()
	assertModelIndex(t, productIndexes, "idx_active_order", false, "ActiveOrderID")

	orderChecks := orderSchema.ParseCheckConstraints()
	orderCheck, ok := orderChecks["chk_orders_quantity_positive"]
	if !ok || normalizeCheckExpression(orderCheck.Constraint) != "quantity>0" {
		t.Fatalf("quantity check = %+v", orderCheck)
	}
	productChecks := productSchema.ParseCheckConstraints()
	productCheck, ok := productChecks["chk_products_stock_reservation_bounds"]
	if !ok ||
		normalizeCheckExpression(productCheck.Constraint) !=
			"stock>=0andreserved_stock>=0andreserved_stock<=stock" {
		t.Fatalf("stock reservation check = %+v", productCheck)
	}
}

func TestCategoryModelMetadataSupportsMerchantOwnership(t *testing.T) {
	categorySchema, err := schema.Parse(&Category{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse category schema: %v", err)
	}

	assertFieldTagContains(t, categorySchema, "MerchantID",
		"index:idx_merchant_parent_sort,priority:1",
		"index:idx_merchant_level_status_sort,priority:1",
	)
	assertFieldTagContains(t, categorySchema, "ParentID",
		"index:idx_merchant_parent_sort,priority:2",
	)

	indexes := categorySchema.ParseIndexes()
	assertModelIndex(t, indexes, "idx_merchant_parent_sort", false, "MerchantID", "ParentID", "Sort")
	assertModelIndex(t, indexes, "idx_merchant_level_status_sort", false, "MerchantID", "Level", "Status", "Sort")
}

func TestAutoMigrateCreatesMultiStockSchemaWithoutLegacyUniqueIndex(t *testing.T) {
	db := newModelSchemaTestDB(t)
	for run := 0; run < 2; run++ {
		if err := db.AutoMigrate(&Product{}, &Order{}); err != nil {
			t.Fatalf("auto migrate run %d: %v", run+1, err)
		}
	}

	for _, indexName := range []string{"idx_product_id", "idx_product_active"} {
		if !db.Migrator().HasIndex(&Order{}, indexName) {
			t.Fatalf("missing order index %s", indexName)
		}
	}
	if db.Migrator().HasIndex(&Order{}, "uk_product_active") {
		t.Fatal("AutoMigrate recreated the invalid uk_product_active index")
	}
	if !db.Migrator().HasIndex(&Product{}, "idx_active_order") {
		t.Fatal("AutoMigrate did not preserve the active_order_id query index")
	}
	for target, constraintName := range map[any]string{
		&Order{}:   "chk_orders_quantity_positive",
		&Product{}: "chk_products_stock_reservation_bounds",
	} {
		if !db.Migrator().HasConstraint(target, constraintName) {
			t.Fatalf("missing check constraint %s", constraintName)
		}
	}

	assertColumnContract(t, db, &Order{}, "quantity", false, "1")
	assertColumnContract(t, db, &Product{}, "stock", false, "1")
	assertColumnContract(t, db, &Product{}, "reserved_stock", false, "0")

	for indexName, unique := range sqliteOrderIndexes(t, db) {
		if indexName == "uk_product_active" {
			t.Fatal("SQLite schema contains the invalid uk_product_active index")
		}
		if (indexName == "idx_product_id" || indexName == "idx_product_active") && unique {
			t.Fatalf("replacement order index %s is unique", indexName)
		}
	}
}

func TestMultiStockSchemaDefaultsAndChecksRemainBackwardCompatible(t *testing.T) {
	db := newModelSchemaTestDB(t)
	if err := db.AutoMigrate(&Product{}, &Order{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	product := Product{ProductNo: "product-defaults"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product with defaults: %v", err)
	}
	var storedProduct Product
	if err := db.First(&storedProduct, product.ID).Error; err != nil {
		t.Fatalf("load product with defaults: %v", err)
	}
	if storedProduct.Stock != 1 || storedProduct.ReservedStock != 0 {
		t.Fatalf(
			"product defaults = stock:%d reserved:%d",
			storedProduct.Stock,
			storedProduct.ReservedStock,
		)
	}

	for _, invalid := range []Product{
		{ProductNo: "negative-stock", Stock: -1},
		{ProductNo: "negative-reserved", Stock: 2, ReservedStock: -1},
		{ProductNo: "reserved-over-stock", Stock: 2, ReservedStock: 3},
	} {
		if err := db.Create(&invalid).Error; err == nil {
			t.Fatalf("invalid stock state was accepted: %+v", invalid)
		}
	}

	for index, active := range []bool{true, true, false, false} {
		order := Order{
			OrderNo:   fmt.Sprintf("order-%d", index),
			ProductID: product.ID,
			IsActive:  active,
		}
		if err := db.Create(&order).Error; err != nil {
			t.Fatalf("create order %d: %v", index, err)
		}
		var stored Order
		if err := db.First(&stored, order.ID).Error; err != nil {
			t.Fatalf("load order %d: %v", index, err)
		}
		if stored.Quantity != 1 {
			t.Fatalf("order %d quantity = %d, want 1", index, stored.Quantity)
		}
	}

	invalidOrder := Order{
		OrderNo:   "invalid-quantity",
		ProductID: product.ID,
		Quantity:  -1,
	}
	if err := db.Create(&invalidOrder).Error; err == nil {
		t.Fatal("non-positive order quantity was accepted")
	}
}

func TestImageBackfillRunPrimaryKeyColumnIsExplicit(t *testing.T) {
	field, ok := reflect.TypeOf(ImageBackfillRun{}).FieldByName("ID")
	if !ok {
		t.Fatal("ImageBackfillRun.ID field is missing")
	}

	tag := field.Tag.Get("gorm")
	if !strings.Contains(tag, "column:id") {
		t.Fatalf("ImageBackfillRun.ID gorm tag must explicitly bind to column:id, got %q", tag)
	}
}

func newModelSchemaTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:model_schema_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open model schema database: %v", err)
	}
	return db
}

func assertFieldTagContains(t *testing.T, parsed *schema.Schema, fieldName string, fragments ...string) {
	t.Helper()
	field := parsed.LookUpField(fieldName)
	if field == nil {
		t.Fatalf("missing model field %s", fieldName)
	}
	tag := field.StructField.Tag.Get("gorm")
	for _, fragment := range fragments {
		if !strings.Contains(tag, fragment) {
			t.Fatalf("%s gorm tag %q does not contain %q", fieldName, tag, fragment)
		}
	}
}

func assertModelIndex(
	t *testing.T,
	indexes []*schema.Index,
	name string,
	unique bool,
	fieldNames ...string,
) {
	t.Helper()
	for _, index := range indexes {
		if index.Name != name {
			continue
		}
		if (index.Class == "UNIQUE") != unique {
			t.Fatalf("index %s class = %q", name, index.Class)
		}
		if len(index.Fields) != len(fieldNames) {
			t.Fatalf("index %s fields = %d, want %d", name, len(index.Fields), len(fieldNames))
		}
		for fieldIndex, expectedName := range fieldNames {
			if index.Fields[fieldIndex].Field.Name != expectedName {
				t.Fatalf(
					"index %s field %d = %s, want %s",
					name,
					fieldIndex,
					index.Fields[fieldIndex].Field.Name,
					expectedName,
				)
			}
		}
		return
	}
	t.Fatalf("missing model index %s", name)
}

func assertColumnContract(
	t *testing.T,
	db *gorm.DB,
	target any,
	columnName string,
	wantNullable bool,
	wantDefault string,
) {
	t.Helper()
	columns, err := db.Migrator().ColumnTypes(target)
	if err != nil {
		t.Fatalf("load columns for %T: %v", target, err)
	}
	for _, column := range columns {
		if column.Name() != columnName {
			continue
		}
		nullable, ok := column.Nullable()
		if !ok || nullable != wantNullable {
			t.Fatalf("%s nullable = %v, known=%v", columnName, nullable, ok)
		}
		defaultValue, ok := column.DefaultValue()
		if !ok || strings.Trim(defaultValue, "'\"()") != wantDefault {
			t.Fatalf("%s default = %q, known=%v", columnName, defaultValue, ok)
		}
		return
	}
	t.Fatalf("missing column %s", columnName)
}

func sqliteOrderIndexes(t *testing.T, db *gorm.DB) map[string]bool {
	t.Helper()
	type indexRow struct {
		Name   string
		Unique int
	}
	var rows []indexRow
	if err := db.Raw("PRAGMA index_list('orders')").Scan(&rows).Error; err != nil {
		t.Fatalf("list SQLite order indexes: %v", err)
	}
	indexes := make(map[string]bool, len(rows))
	for _, row := range rows {
		indexes[row.Name] = row.Unique == 1
	}
	return indexes
}

func normalizeCheckExpression(value string) string {
	replacer := strings.NewReplacer("`", "", " ", "", "(", "", ")", "")
	return strings.ToLower(replacer.Replace(value))
}
