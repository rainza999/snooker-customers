package inventory

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	model "github.com/rainza999/fiber-test/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupInventoryTestDB(t *testing.T) (*gorm.DB, model.Product) {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "stock.db")+"?_busy_timeout=5000"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := database.DB()
		_ = sqlDB.Close()
	})
	if err := database.AutoMigrate(&model.Category{}, &model.Product{}, &model.Supplier{}, &model.ProductReceipt{}, &model.ProductReceiptItem{}); err != nil {
		t.Fatalf("migrate product tables: %v", err)
	}
	if err := Migrate(database); err != nil {
		t.Fatalf("migrate inventory tables: %v", err)
	}
	category := model.Category{Name: "Stock", IsStock: 1, IsActive: 1}
	if err := database.Create(&category).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}
	product := model.Product{Name: "Cola", CategoryID: category.ID, Unit: "bottle", Price: 20, IsActive: true}
	if err := database.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	return database, product
}

func TestMigrateIsIdempotentForFreshDatabase(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "fresh.db")+"?_busy_timeout=5000"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := database.DB()
		_ = sqlDB.Close()
	})
	if err := Migrate(database); err != nil {
		t.Fatalf("first migration: %v", err)
	}
	if err := Migrate(database); err != nil {
		t.Fatalf("second migration: %v", err)
	}
}

func createStockEntry(t *testing.T, database *gorm.DB, productID uint, quantity int, cost float64, date time.Time) model.StockEntry {
	t.Helper()
	entry := model.StockEntry{
		ProductID:       productID,
		StockLocationID: 1,
		Quantity:        quantity,
		RemainingQty:    quantity,
		CostPerUnit:     cost,
		EntryDate:       date,
	}
	if err := database.Create(&entry).Error; err != nil {
		t.Fatalf("create stock entry: %v", err)
	}
	return entry
}

func TestDeductFIFOInsufficientStockRollsBack(t *testing.T) {
	database, product := setupInventoryTestDB(t)
	entry := createStockEntry(t, database, product.ID, 2, 10, time.Now())

	err := WithTransaction(database, func(tx *gorm.DB) error {
		_, err := DeductFIFO(tx, product.ID, 1, 1, 3)
		return err
	})
	if !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("expected insufficient stock, got %v", err)
	}
	if err := database.First(&entry, entry.ID).Error; err != nil {
		t.Fatalf("reload entry: %v", err)
	}
	if entry.RemainingQty != 2 {
		t.Fatalf("stock changed after failed deduction: got %d want 2", entry.RemainingQty)
	}
	var movementCount int64
	if err := database.Model(&model.InventoryTransaction{}).Count(&movementCount).Error; err != nil {
		t.Fatalf("count movements: %v", err)
	}
	if movementCount != 0 {
		t.Fatalf("unexpected movements after rollback: %d", movementCount)
	}
}

func TestDeductAndReturnFIFORecordsExactLots(t *testing.T) {
	database, product := setupInventoryTestDB(t)
	first := createStockEntry(t, database, product.ID, 2, 10, time.Now().Add(-time.Hour))
	second := createStockEntry(t, database, product.ID, 2, 20, time.Now())

	err := WithTransaction(database, func(tx *gorm.DB) error {
		cost, err := DeductFIFO(tx, product.ID, 12, 34, 3)
		if err != nil {
			return err
		}
		if cost != 40 {
			t.Fatalf("deduct cost: got %.2f want 40", cost)
		}
		returnedCost, err := ReturnFIFO(tx, product.ID, 12, 34, 2)
		if err != nil {
			return err
		}
		if returnedCost != 30 {
			t.Fatalf("return cost: got %.2f want 30", returnedCost)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("deduct and return: %v", err)
	}
	if err := database.First(&first, first.ID).Error; err != nil {
		t.Fatalf("reload first: %v", err)
	}
	if err := database.First(&second, second.ID).Error; err != nil {
		t.Fatalf("reload second: %v", err)
	}
	if first.RemainingQty != 1 || second.RemainingQty != 2 {
		t.Fatalf("unexpected remaining quantities: first=%d second=%d", first.RemainingQty, second.RemainingQty)
	}
	var movementCount int64
	if err := database.Model(&model.InventoryTransaction{}).Count(&movementCount).Error; err != nil {
		t.Fatalf("count movements: %v", err)
	}
	if movementCount != 4 {
		t.Fatalf("movement count: got %d want 4", movementCount)
	}
}

func TestConcurrentDeductOnlyOneRequestGetsLastItem(t *testing.T) {
	database, product := setupInventoryTestDB(t)
	entry := createStockEntry(t, database, product.ID, 1, 10, time.Now())

	start := make(chan struct{})
	results := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for index := 0; index < 2; index++ {
		waitGroup.Add(1)
		go func(serviceID uint) {
			defer waitGroup.Done()
			<-start
			results <- WithTransaction(database, func(tx *gorm.DB) error {
				_, err := DeductFIFO(tx, product.ID, serviceID, 1, 1)
				return err
			})
		}(uint(index + 1))
	}
	close(start)
	waitGroup.Wait()
	close(results)

	successes := 0
	conflicts := 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrInsufficientStock) {
			conflicts++
		} else {
			t.Fatalf("unexpected deduction error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("unexpected results: successes=%d conflicts=%d", successes, conflicts)
	}
	if err := database.First(&entry, entry.ID).Error; err != nil {
		t.Fatalf("reload entry: %v", err)
	}
	if entry.RemainingQty != 0 {
		t.Fatalf("remaining stock: got %d want 0", entry.RemainingQty)
	}
}

func TestCreateReceiptStockRejectsDuplicateFinalize(t *testing.T) {
	database, product := setupInventoryTestDB(t)
	receipt := model.ProductReceipt{SupplierID: 1, ReceiptNumber: "INV-001", ReceivedDate: time.Now(), ReceiptStatus: "draft", IsActive: 1}
	if err := database.Create(&receipt).Error; err != nil {
		t.Fatalf("create receipt: %v", err)
	}
	item := model.ProductReceiptItem{ReceiptID: receipt.ID, ProductID: product.ID, Quantity: 2, TotalPrice: 20, UnitPrice: 10, ReceiptItemStatus: "draft", IsActive: 1}
	if err := database.Create(&item).Error; err != nil {
		t.Fatalf("create receipt item: %v", err)
	}
	if err := WithTransaction(database, func(tx *gorm.DB) error {
		_, err := CreateReceiptStock(tx, item)
		return err
	}); err != nil {
		t.Fatalf("create receipt stock: %v", err)
	}
	if err := WithTransaction(database, func(tx *gorm.DB) error {
		_, err := CreateReceiptStock(tx, item)
		return err
	}); err == nil {
		t.Fatal("expected duplicate receipt stock to fail")
	}
	var count int64
	if err := database.Model(&model.StockEntry{}).Count(&count).Error; err != nil {
		t.Fatalf("count entries: %v", err)
	}
	if count != 1 {
		t.Fatalf("stock entry count: got %d want 1", count)
	}
}

func TestAdjustStockRequiresNoteAndRecordsLedger(t *testing.T) {
	database, product := setupInventoryTestDB(t)
	if err := AdjustStock(database, product.ID, 3, 12, "physical count correction"); err != nil {
		t.Fatalf("increase adjustment: %v", err)
	}
	if err := AdjustStock(database, product.ID, -1, 0, "damaged item"); err != nil {
		t.Fatalf("decrease adjustment: %v", err)
	}
	if err := AdjustStock(database, product.ID, 1, 10, ""); err == nil {
		t.Fatal("expected missing note to fail")
	}
	var remaining int
	if err := database.Model(&model.StockEntry{}).Select("COALESCE(SUM(remaining_qty), 0)").Scan(&remaining).Error; err != nil {
		t.Fatalf("sum remaining: %v", err)
	}
	if remaining != 2 {
		t.Fatalf("remaining stock: got %d want 2", remaining)
	}
	var movementCount int64
	if err := database.Model(&model.InventoryTransaction{}).Count(&movementCount).Error; err != nil {
		t.Fatalf("count movements: %v", err)
	}
	if movementCount != 2 {
		t.Fatalf("movement count: got %d want 2", movementCount)
	}
}
