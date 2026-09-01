package saleofreport

import (
	"encoding/json"
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

type monthlyCloseReportResponse struct {
	Report []struct {
		Date     string  `json:"date"`
		GameFee  float64 `json:"game_fee"`
		DrinkFee float64 `json:"drink_fee"`
		TotalFee float64 `json:"total_fee"`
	} `json:"report"`
}

type dailyCloseReportResponse struct {
	Visitations []struct {
		BillNumber string  `json:"bill_number"`
		TableName  string  `json:"table_name"`
		TotalBill  float64 `json:"total_bill"`
		Uuid       string  `json:"uuid"`
	} `json:"visitations"`
}

func setupSalesCloseReportTest(t *testing.T) (*fiber.App, *gorm.DB, uint, uint, uint) {
	t.Helper()

	originalDB := db.Db
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "sales-close-report.db")+"?_busy_timeout=5000"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := database.AutoMigrate(
		&model.Division{},
		&model.SettingTable{},
		&model.Product{},
		&model.Visitation{},
		&model.Service{},
	); err != nil {
		t.Fatalf("migrate report tables: %v", err)
	}

	division := model.Division{Code: "01", MaxDigits: "000000", Name: "Test", IsActive: 1}
	if err := database.Create(&division).Error; err != nil {
		t.Fatalf("create division: %v", err)
	}
	table := model.SettingTable{DivisionID: division.ID, Name: "โต๊ะ 1", Price: 140, Price2: 70, IsActive: 1}
	if err := database.Create(&table).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	game := model.Product{Name: "Table time", Unit: "hour", Price: 140, IsSnookerTime: true, IsActive: true}
	if err := database.Create(&game).Error; err != nil {
		t.Fatalf("create game product: %v", err)
	}
	if game.ID != 1 {
		t.Fatalf("game product id: got %d want 1", game.ID)
	}
	drink := model.Product{Name: "Water", CategoryID: 1, Unit: "bottle", Price: 20, IsActive: true}
	if err := database.Create(&drink).Error; err != nil {
		t.Fatalf("create drink product: %v", err)
	}

	db.Db = database
	t.Cleanup(func() {
		sqlDB, _ := database.DB()
		_ = sqlDB.Close()
		db.Db = originalDB
	})

	app := fiber.New()
	app.Get("/sale-report-closes/daily", GetDailySalesCloseReport)
	app.Get("/sale-report-closes/monthly", GetMonthlySalesCloseReport)
	return app, database, division.ID, table.ID, drink.ID
}

func createCloseReportVisitation(t *testing.T, database *gorm.DB, divisionID, tableID uint, startTime time.Time, isActive, isPaid uint8) model.Visitation {
	t.Helper()

	visitation := model.Visitation{DivisionID: divisionID, TableID: tableID}
	if err := database.Create(&visitation).Error; err != nil {
		t.Fatalf("create visitation: %v", err)
	}
	updates := map[string]interface{}{
		"start_time": startTime,
		"end_time":   startTime.Add(time.Hour),
		"is_active":  isActive,
		"is_paid":    isPaid,
	}
	if err := database.Model(&visitation).Updates(updates).Error; err != nil {
		t.Fatalf("update visitation state: %v", err)
	}
	if err := database.First(&visitation, visitation.ID).Error; err != nil {
		t.Fatalf("reload visitation: %v", err)
	}
	return visitation
}

func createCloseReportService(t *testing.T, database *gorm.DB, visitationID, productID uint, status string, netPrice float64) {
	t.Helper()

	service := model.Service{
		VisitationID: visitationID,
		ProductID:    productID,
		SellQuantity: 1,
		SellUnitID:   "unit",
		TotalCost:    netPrice,
		NetPrice:     netPrice,
		Status:       status,
	}
	if err := database.Create(&service).Error; err != nil {
		t.Fatalf("create service: %v", err)
	}
}

func seedClosedTableReportRows(t *testing.T, database *gorm.DB, divisionID, tableID, drinkID uint, at time.Time) model.Visitation {
	t.Helper()

	unpaidClosed := createCloseReportVisitation(t, database, divisionID, tableID, at, 0, 0)
	createCloseReportService(t, database, unpaidClosed.ID, 1, "draft", 140)
	createCloseReportService(t, database, unpaidClosed.ID, drinkID, "draft", 20)

	paidClosed := createCloseReportVisitation(t, database, divisionID, tableID, at.Add(time.Hour), 0, 1)
	createCloseReportService(t, database, paidClosed.ID, 1, "paid", 999)

	pendingClosed := createCloseReportVisitation(t, database, divisionID, tableID, at.Add(2*time.Hour), 0, 2)
	createCloseReportService(t, database, pendingClosed.ID, 1, "draft", 888)

	openDraft := createCloseReportVisitation(t, database, divisionID, tableID, at.Add(3*time.Hour), 1, 0)
	createCloseReportService(t, database, openDraft.ID, 1, "draft", 777)

	return unpaidClosed
}

func TestGetDailySalesCloseReportCountsClosedUnpaidTablesOnly(t *testing.T) {
	app, database, divisionID, tableID, drinkID := setupSalesCloseReportTest(t)
	closedTable := seedClosedTableReportRows(t, database, divisionID, tableID, drinkID, time.Date(2026, 8, 31, 14, 30, 0, 0, time.UTC))

	request := httptest.NewRequest("GET", "/sale-report-closes/daily?start_date=2026-08-31&end_date=2026-08-31", nil)
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatalf("daily close report request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("status: got %d want %d", response.StatusCode, fiber.StatusOK)
	}

	var body dailyCloseReportResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Visitations) != 1 {
		t.Fatalf("visitations: got %d want 1, %#v", len(body.Visitations), body.Visitations)
	}
	if body.Visitations[0].Uuid != closedTable.Uuid {
		t.Fatalf("uuid: got %q want %q", body.Visitations[0].Uuid, closedTable.Uuid)
	}
	if body.Visitations[0].TotalBill != 160 {
		t.Fatalf("total bill: got %.2f want 160.00", body.Visitations[0].TotalBill)
	}
}

func TestGetMonthlySalesCloseReportCountsClosedUnpaidTablesOnlyAcrossFullMonth(t *testing.T) {
	app, database, divisionID, tableID, drinkID := setupSalesCloseReportTest(t)
	lastDay := time.Date(2026, 8, 31, 14, 30, 0, 0, time.UTC)
	seedClosedTableReportRows(t, database, divisionID, tableID, drinkID, lastDay)

	request := httptest.NewRequest("GET", "/sale-report-closes/monthly?month=2026-08", nil)
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatalf("monthly close report request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("status: got %d want %d", response.StatusCode, fiber.StatusOK)
	}

	var body monthlyCloseReportResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Report) != 1 {
		t.Fatalf("report rows: got %d want 1, %#v", len(body.Report), body.Report)
	}
	row := body.Report[0]
	if row.Date != "2026-08-31" {
		t.Fatalf("date: got %q want 2026-08-31", row.Date)
	}
	if row.GameFee != 140 {
		t.Fatalf("game fee: got %.2f want 140.00", row.GameFee)
	}
	if row.DrinkFee != 20 {
		t.Fatalf("drink fee: got %.2f want 20.00", row.DrinkFee)
	}
	if row.TotalFee != 160 {
		t.Fatalf("total fee: got %.2f want 160.00", row.TotalFee)
	}
}
