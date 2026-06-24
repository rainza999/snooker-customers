package receipt

import (
	"bytes"
	"fmt"
	"io"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	"github.com/rainza999/fiber-test/inventory"
	model "github.com/rainza999/fiber-test/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupReceiptTestDB(t *testing.T) (*fiber.App, model.Supplier, model.Product) {
	t.Helper()
	originalDB := db.Db
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "receipt.db")+"?_busy_timeout=5000"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := database.AutoMigrate(&model.Supplier{}, &model.Product{}, &model.ProductReceipt{}, &model.ProductReceiptItem{}); err != nil {
		t.Fatalf("migrate base tables: %v", err)
	}
	if err := inventory.Migrate(database); err != nil {
		t.Fatalf("migrate inventory: %v", err)
	}
	supplier := model.Supplier{Name: "Supplier A", IsActive: true}
	if err := database.Create(&supplier).Error; err != nil {
		t.Fatalf("create supplier: %v", err)
	}
	product := model.Product{Name: "Cola", Unit: "bottle", Price: 20, IsActive: true}
	if err := database.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	db.Db = database
	t.Cleanup(func() {
		sqlDB, _ := database.DB()
		_ = sqlDB.Close()
		db.Db = originalDB
	})

	app := fiber.New()
	app.Post("/receipts/submit", SubmitReceipt)
	app.Post("/receipts/finalize", FinalizeReceipt)
	app.Delete("/receipts/:id/delete", DeleteReceipt)
	return app, supplier, product
}

func postJSON(t *testing.T, app *fiber.App, method, path, body string) int {
	t.Helper()
	status, _ := postJSONResponse(t, app, method, path, body)
	return status
}

func postJSONResponse(t *testing.T, app *fiber.App, method, path, body string) (int, string) {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return response.StatusCode, string(responseBody)
}

func TestFinalizeReceiptConcurrentRequestCreatesOneStockLot(t *testing.T) {
	app, supplier, product := setupReceiptTestDB(t)
	receipt := model.ProductReceipt{SupplierID: supplier.ID, ReceiptNumber: "INV-001", ReceiptStatus: "draft", IsActive: 1}
	if err := db.Db.Create(&receipt).Error; err != nil {
		t.Fatalf("create receipt: %v", err)
	}
	item := model.ProductReceiptItem{ReceiptID: receipt.ID, ProductID: product.ID, Quantity: 2, UnitPrice: 10, TotalPrice: 20, ReceiptItemStatus: "draft", IsActive: 1}
	if err := db.Db.Create(&item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}

	body := fmt.Sprintf(`{"receipts":[{"receipt_id":%d}]}`, receipt.ID)
	start := make(chan struct{})
	statuses := make(chan int, 2)
	var waitGroup sync.WaitGroup
	for index := 0; index < 2; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			request := httptest.NewRequest("POST", "/receipts/finalize", bytes.NewBufferString(body))
			request.Header.Set("Content-Type", "application/json")
			response, err := app.Test(request, -1)
			if err != nil {
				t.Errorf("finalize receipt: %v", err)
				return
			}
			defer response.Body.Close()
			statuses <- response.StatusCode
		}()
	}
	close(start)
	waitGroup.Wait()
	close(statuses)

	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[fiber.StatusOK] != 1 || counts[fiber.StatusConflict] != 1 {
		t.Fatalf("expected one 200 and one 409, got %#v", counts)
	}
	var stockCount int64
	if err := db.Db.Model(&model.StockEntry{}).Count(&stockCount).Error; err != nil {
		t.Fatalf("count stock entries: %v", err)
	}
	if stockCount != 1 {
		t.Fatalf("stock lot count: got %d want 1", stockCount)
	}
	var movementCount int64
	if err := db.Db.Model(&model.InventoryTransaction{}).Count(&movementCount).Error; err != nil {
		t.Fatalf("count movements: %v", err)
	}
	if movementCount != 1 {
		t.Fatalf("movement count: got %d want 1", movementCount)
	}
}

func TestSubmitReceiptAllowsSameNumberForDifferentSuppliers(t *testing.T) {
	app, firstSupplier, product := setupReceiptTestDB(t)
	secondSupplier := model.Supplier{Name: "Supplier B", IsActive: true}
	if err := db.Db.Create(&secondSupplier).Error; err != nil {
		t.Fatalf("create second supplier: %v", err)
	}
	template := `{"drafts":{"supplier":%d,"product":%d,"quantity":1,"totalPrice":10,"purchaseOrderNumber":"INV-SAME","status":"draft"}}`
	if status := postJSON(t, app, "POST", "/receipts/submit", fmt.Sprintf(template, firstSupplier.ID, product.ID)); status != fiber.StatusOK {
		t.Fatalf("first submit status: %d", status)
	}
	if status := postJSON(t, app, "POST", "/receipts/submit", fmt.Sprintf(template, secondSupplier.ID, product.ID)); status != fiber.StatusOK {
		t.Fatalf("second submit status: %d", status)
	}
	var count int64
	if err := db.Db.Model(&model.ProductReceipt{}).Where("receipt_number = ?", "INV-SAME").Count(&count).Error; err != nil {
		t.Fatalf("count receipts: %v", err)
	}
	if count != 2 {
		t.Fatalf("receipt count: got %d want 2", count)
	}
}

func TestSubmitReceiptRejectsSavedNumberForSameSupplierWithFriendlyMessage(t *testing.T) {
	app, supplier, product := setupReceiptTestDB(t)
	receipt := model.ProductReceipt{SupplierID: supplier.ID, ReceiptNumber: "INV-SAVED", ReceiptStatus: "save", IsActive: 1}
	if err := db.Db.Create(&receipt).Error; err != nil {
		t.Fatalf("create saved receipt: %v", err)
	}

	body := fmt.Sprintf(
		`{"drafts":{"supplier":%d,"product":%d,"quantity":1,"totalPrice":10,"purchaseOrderNumber":"INV-SAVED","status":"draft"}}`,
		supplier.ID,
		product.ID,
	)
	status, responseBody := postJSONResponse(t, app, "POST", "/receipts/submit", body)
	if status != fiber.StatusConflict {
		t.Fatalf("submit status: got %d want 409; body=%s", status, responseBody)
	}
	if !strings.Contains(responseBody, "receipt_number_already_exists_for_supplier") {
		t.Fatalf("response body missing duplicate error code: %s", responseBody)
	}
	if !strings.Contains(responseBody, receiptNumberAlreadyExistsForSupplierMessage) {
		t.Fatalf("response body missing friendly message: %s", responseBody)
	}
}

func TestDeleteReceiptRejectsSavedItem(t *testing.T) {
	app, supplier, product := setupReceiptTestDB(t)
	receipt := model.ProductReceipt{SupplierID: supplier.ID, ReceiptNumber: "INV-SAVED", ReceiptStatus: "save", IsActive: 1}
	if err := db.Db.Create(&receipt).Error; err != nil {
		t.Fatalf("create receipt: %v", err)
	}
	item := model.ProductReceiptItem{ReceiptID: receipt.ID, ProductID: product.ID, Quantity: 1, UnitPrice: 10, TotalPrice: 10, ReceiptItemStatus: "save", IsActive: 1}
	if err := db.Db.Create(&item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}
	if status := postJSON(t, app, "DELETE", fmt.Sprintf("/receipts/%d/delete", item.ID), ""); status != fiber.StatusConflict {
		t.Fatalf("delete status: got %d want 409", status)
	}
	var count int64
	if err := db.Db.Model(&model.ProductReceiptItem{}).Where("id = ?", item.ID).Count(&count).Error; err != nil {
		t.Fatalf("count item: %v", err)
	}
	if count != 1 {
		t.Fatalf("saved item was deleted")
	}
}
