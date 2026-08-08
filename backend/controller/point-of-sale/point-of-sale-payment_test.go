package pointofsale

import (
	"bytes"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPaymentTest(t *testing.T) (*fiber.App, model.Visitation, model.Service, model.Product) {
	t.Helper()
	originalDB := db.Db
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "payment.db")+"?_busy_timeout=5000"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := database.AutoMigrate(&model.Division{}, &model.Product{}, &model.Visitation{}, &model.Service{}); err != nil {
		t.Fatalf("migrate payment tables: %v", err)
	}
	division := model.Division{Code: "01", MaxDigits: "000000", Name: "Test", IsActive: 1}
	if err := database.Create(&division).Error; err != nil {
		t.Fatalf("create division: %v", err)
	}
	game := model.Product{Name: "Table time", Unit: "hour", Price: 100, IsSnookerTime: true, IsActive: true}
	food := model.Product{Name: "Cola", Unit: "bottle", Price: 20, IsActive: true}
	if err := database.Create(&game).Error; err != nil {
		t.Fatalf("create game product: %v", err)
	}
	if err := database.Create(&food).Error; err != nil {
		t.Fatalf("create food product: %v", err)
	}
	visitation := model.Visitation{DivisionID: division.ID, StartTime: time.Now().Add(-time.Hour), IsActive: 1, IsRunning: 1}
	if err := database.Create(&visitation).Error; err != nil {
		t.Fatalf("create visitation: %v", err)
	}
	if err := database.Create(&model.Service{VisitationID: visitation.ID, ProductID: game.ID, SellQuantity: 1, TotalCost: 100, NetPrice: 100, Status: "draft"}).Error; err != nil {
		t.Fatalf("create game service: %v", err)
	}
	foodService := model.Service{VisitationID: visitation.ID, ProductID: food.ID, SellQuantity: 2, TotalCost: 40, NetPrice: 40, TotalFIFO_Cost: 20, Status: "draft"}
	if err := database.Create(&foodService).Error; err != nil {
		t.Fatalf("create food service: %v", err)
	}
	db.Db = database
	t.Cleanup(func() {
		sqlDB, _ := database.DB()
		_ = sqlDB.Close()
		db.Db = originalDB
	})
	app := fiber.New()
	app.Put("/point-of-sales/:uuid/visitation/payment", PaymentStore)
	return app, visitation, foodService, food
}

func paymentBody(visitation model.Visitation, food model.Product, quantity int) string {
	return fmt.Sprintf(`{
		"uuid":%q,
		"total_cost":"140.00",
		"net_price":"140.00",
		"paid_amount":"200.00",
		"is_paid":1,
		"table_type":0,
		"end_time":%q,
		"services":[
			{"product_id":1,"sell_quantity":1,"total_cost":"100.00","net_price":"100.00"},
			{"product_id":%d,"sell_quantity":%d,"total_cost":"40.00","net_price":"40.00"}
		]
	}`, visitation.Uuid, time.Now().Format(time.RFC3339), food.ID, quantity)
}

func putPayment(t *testing.T, app *fiber.App, visitation model.Visitation, body string) int {
	t.Helper()
	request := httptest.NewRequest("PUT", "/point-of-sales/"+visitation.Uuid+"/visitation/payment", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatalf("payment request: %v", err)
	}
	defer response.Body.Close()
	return response.StatusCode
}

func TestPaymentStoreRejectsStaleFoodQuantity(t *testing.T) {
	app, visitation, foodService, food := setupPaymentTest(t)
	if status := putPayment(t, app, visitation, paymentBody(visitation, food, 3)); status != fiber.StatusConflict {
		t.Fatalf("payment status: got %d want 409", status)
	}
	if err := db.Db.First(&foodService, foodService.ID).Error; err != nil {
		t.Fatalf("reload food service: %v", err)
	}
	if foodService.SellQuantity != 2 || foodService.Status != "draft" {
		t.Fatalf("food service mutated after stale payment: %#v", foodService)
	}
}

func TestPaymentStoreMarksPersistedServicesPaidWithoutOverwritingQuantity(t *testing.T) {
	app, visitation, foodService, food := setupPaymentTest(t)
	if status := putPayment(t, app, visitation, paymentBody(visitation, food, 2)); status != fiber.StatusOK {
		t.Fatalf("payment status: got %d want 200", status)
	}
	if err := db.Db.First(&foodService, foodService.ID).Error; err != nil {
		t.Fatalf("reload food service: %v", err)
	}
	if foodService.SellQuantity != 2 || foodService.Status != "paid" {
		t.Fatalf("unexpected food service after payment: %#v", foodService)
	}
	var count int64
	if err := db.Db.Model(&model.Service{}).Where("visitation_id = ?", visitation.ID).Count(&count).Error; err != nil {
		t.Fatalf("count services: %v", err)
	}
	if count != 2 {
		t.Fatalf("service count: got %d want 2", count)
	}

	var paidVisitation model.Visitation
	if err := db.Db.First(&paidVisitation, visitation.ID).Error; err != nil {
		t.Fatalf("reload visitation: %v", err)
	}
	if paidVisitation.IsPaid != 1 || paidVisitation.IsActive != 0 || paidVisitation.IsRunning != 0 {
		t.Fatalf("visitation should be paid and closed after payment: %#v", paidVisitation)
	}
}
