package pointofsale

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPOSTestDB(t *testing.T) (uint, uint, uint) {
	t.Helper()

	originalDB := db.Db
	testDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "snooker.db")+"?_busy_timeout=5000"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := testDB.AutoMigrate(
		&model.Division{},
		&model.User{},
		&model.SettingTable{},
		&model.Category{},
		&model.Product{},
		&model.Visitation{},
		&model.Service{},
	); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	division := model.Division{Code: "01", MaxDigits: "000000", Name: "Test", IsActive: 1}
	if err := testDB.Create(&division).Error; err != nil {
		t.Fatalf("create division: %v", err)
	}
	user := model.User{DivisionID: division.ID, Username: "test", IsActive: 1}
	if err := testDB.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := testDB.Create(&model.Product{Name: "Table time", Unit: "hour", Price: 100, IsSnookerTime: true}).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}

	tableIDs := make([]uint, 0, 3)
	for index := 1; index <= 3; index++ {
		table := model.SettingTable{
			DivisionID: division.ID,
			Code:       fmt.Sprintf("T%d", index),
			Name:       fmt.Sprintf("Table %d", index),
			Price:      100,
			IsActive:   1,
		}
		if err := testDB.Create(&table).Error; err != nil {
			t.Fatalf("create table: %v", err)
		}
		tableIDs = append(tableIDs, table.ID)
	}

	db.Db = testDB
	t.Cleanup(func() {
		sqlDB, _ := testDB.DB()
		_ = sqlDB.Close()
		db.Db = originalDB
	})

	return user.ID, tableIDs[0], tableIDs[1]
}

func newPOSTestApp(userID uint) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("userID", int(userID))
		return c.Next()
	})
	app.Post("/point-of-sales/store/visitation", Store)
	app.Get("/point-of-sales/anyData", AnyData)
	app.Put("/point-of-sales/:uuid/visitation/changeTable", ChangeTable)
	app.Post("/point-of-sales/:uuid/visitation/order/store", OrderStore)
	return app
}

func postOpenTable(app *fiber.App, tableID uint) (int, error) {
	request := httptest.NewRequest(
		"POST",
		"/point-of-sales/store/visitation",
		bytes.NewBufferString(fmt.Sprintf(`{"tableID":%d,"status":"open"}`, tableID)),
	)
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request, -1)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	return response.StatusCode, nil
}

func createCloseTableUser(t *testing.T, password string) model.Employee {
	t.Helper()

	var division model.Division
	if err := db.Db.First(&division).Error; err != nil {
		t.Fatalf("find division: %v", err)
	}
	employee := model.Employee{FirstName: "Closer", LastName: "User", IsActive: 1}
	if err := db.Db.Create(&employee).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := model.User{
		EmployeeID: employee.ID,
		DivisionID: division.ID,
		Username:   "closer",
		Password:   string(hash),
		IsActive:   1,
	}
	if err := db.Db.Create(&user).Error; err != nil {
		t.Fatalf("create close table user: %v", err)
	}
	return employee
}

func TestStoreRejectsConcurrentOpenForSameTable(t *testing.T) {
	userID, tableID, _ := setupPOSTestDB(t)
	app := newPOSTestApp(userID)

	start := make(chan struct{})
	statuses := make(chan int, 2)
	var waitGroup sync.WaitGroup

	for index := 0; index < 2; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			status, err := postOpenTable(app, tableID)
			if err != nil {
				t.Errorf("open table request: %v", err)
				return
			}
			statuses <- status
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

	var activeCount int64
	if err := db.Db.Model(&model.Visitation{}).
		Where("table_id = ? AND is_active = 1", tableID).
		Count(&activeCount).Error; err != nil {
		t.Fatalf("count active visitations: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("expected one active visitation, got %d", activeCount)
	}
}

func TestVerifyPasswordAndCloseTableLeavesUnpaidClosedDraftData(t *testing.T) {
	userID, tableID, _ := setupPOSTestDB(t)
	app := newPOSTestApp(userID)
	app.Post("/point-of-sales/api/verify-password-and-close-table", VerifyPasswordAndCloseTable)
	employee := createCloseTableUser(t, "1234")

	if status, err := postOpenTable(app, tableID); err != nil || status != fiber.StatusOK {
		t.Fatalf("open table: status=%d err=%v", status, err)
	}

	var opened model.Visitation
	if err := db.Db.Where("table_id = ? AND is_active = 1", tableID).First(&opened).Error; err != nil {
		t.Fatalf("find opened visitation: %v", err)
	}

	body := fmt.Sprintf(`{
		"uuidTable":%q,
		"uuid":%q,
		"password":"1234",
		"tableID":%d
	}`, opened.Uuid, employee.Uuid, tableID)
	request := httptest.NewRequest(
		"POST",
		"/point-of-sales/api/verify-password-and-close-table",
		bytes.NewBufferString(body),
	)
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatalf("close table request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("close table status: got %d want %d", response.StatusCode, fiber.StatusOK)
	}

	var closed model.Visitation
	if err := db.Db.First(&closed, opened.ID).Error; err != nil {
		t.Fatalf("reload closed visitation: %v", err)
	}
	var gameService model.Service
	if err := db.Db.Where("visitation_id = ? AND product_id = 1", closed.ID).First(&gameService).Error; err != nil {
		t.Fatalf("find game service: %v", err)
	}

	t.Logf("closed via UI endpoint: visitation_id=%d is_active=%d is_paid=%d is_running=%d use_time=%d bill_code=%q service_status=%q service_net_price=%.2f",
		closed.ID, closed.IsActive, closed.IsPaid, closed.IsRunning, closed.UseTime, closed.BillCode, gameService.Status, gameService.NetPrice)

	if closed.IsActive != 0 {
		t.Fatalf("is_active after close table: got %d want 0", closed.IsActive)
	}
	if closed.IsPaid != 0 {
		t.Fatalf("is_paid after close table: got %d want 0", closed.IsPaid)
	}
	if closed.IsRunning != 0 {
		t.Fatalf("is_running after close table: got %d want 0", closed.IsRunning)
	}
	if gameService.Status != "draft" {
		t.Fatalf("game service status after close table: got %q want draft", gameService.Status)
	}
}

func TestAnyDataOnlyReturnsActiveTables(t *testing.T) {
	userID, tableID, _ := setupPOSTestDB(t)
	app := newPOSTestApp(userID)

	if err := db.Db.Model(&model.SettingTable{}).
		Where("id <> ?", tableID).
		Update("is_active", 0).Error; err != nil {
		t.Fatalf("deactivate tables: %v", err)
	}

	request := httptest.NewRequest("GET", "/point-of-sales/anyData", nil)
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatalf("list tables request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", response.StatusCode)
	}

	var payload []struct {
		ID       uint  `json:"ID"`
		IsActive uint8 `json:"IsActive"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("expected only active table, got %#v", payload)
	}
	if payload[0].ID != tableID || payload[0].IsActive != 1 {
		t.Fatalf("unexpected table payload: %#v", payload[0])
	}
}

func TestChangeTableRejectsActiveDestination(t *testing.T) {
	userID, sourceTableID, destinationTableID := setupPOSTestDB(t)
	app := newPOSTestApp(userID)

	if status, err := postOpenTable(app, sourceTableID); err != nil || status != fiber.StatusOK {
		t.Fatalf("open source table: status=%d err=%v", status, err)
	}
	if status, err := postOpenTable(app, destinationTableID); err != nil || status != fiber.StatusOK {
		t.Fatalf("open destination table: status=%d err=%v", status, err)
	}

	var source model.Visitation
	if err := db.Db.Where("table_id = ? AND is_active = 1", sourceTableID).First(&source).Error; err != nil {
		t.Fatalf("find source visitation: %v", err)
	}

	request := httptest.NewRequest(
		"PUT",
		fmt.Sprintf("/point-of-sales/%s/visitation/changeTable", source.Uuid),
		bytes.NewBufferString(fmt.Sprintf(`{"newTableID":%d}`, destinationTableID)),
	)
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatalf("move table request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != fiber.StatusConflict {
		t.Fatalf("expected 409, got %d", response.StatusCode)
	}

	var refreshedSource model.Visitation
	if err := db.Db.First(&refreshedSource, source.ID).Error; err != nil {
		t.Fatalf("reload source visitation: %v", err)
	}
	if refreshedSource.TableID != sourceTableID {
		t.Fatalf("source table moved unexpectedly: got %d want %d", refreshedSource.TableID, sourceTableID)
	}

	var availableTable model.SettingTable
	if err := db.Db.Where("code = ?", "T3").First(&availableTable).Error; err != nil {
		t.Fatalf("find available table: %v", err)
	}

	request = httptest.NewRequest(
		"PUT",
		fmt.Sprintf("/point-of-sales/%s/visitation/changeTable", source.Uuid),
		bytes.NewBufferString(fmt.Sprintf(`{"newTableID":%d}`, availableTable.ID)),
	)
	request.Header.Set("Content-Type", "application/json")
	response, err = app.Test(request, -1)
	if err != nil {
		t.Fatalf("move to available table request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("expected successful move, got %d", response.StatusCode)
	}
	if err := db.Db.First(&refreshedSource, source.ID).Error; err != nil {
		t.Fatalf("reload moved visitation: %v", err)
	}
	if refreshedSource.TableID != availableTable.ID {
		t.Fatalf("source table was not moved: got %d want %d", refreshedSource.TableID, availableTable.ID)
	}
}

func TestChangeTableRejectsDifferentTablePrice(t *testing.T) {
	userID, sourceTableID, destinationTableID := setupPOSTestDB(t)
	app := newPOSTestApp(userID)

	if status, err := postOpenTable(app, sourceTableID); err != nil || status != fiber.StatusOK {
		t.Fatalf("open source table: status=%d err=%v", status, err)
	}
	if err := db.Db.Model(&model.SettingTable{}).
		Where("id = ?", destinationTableID).
		Updates(map[string]interface{}{"price": 150, "price2": 100}).Error; err != nil {
		t.Fatalf("update destination price: %v", err)
	}

	var source model.Visitation
	if err := db.Db.Where("table_id = ? AND is_active = 1", sourceTableID).First(&source).Error; err != nil {
		t.Fatalf("find source visitation: %v", err)
	}

	request := httptest.NewRequest(
		"PUT",
		fmt.Sprintf("/point-of-sales/%s/visitation/changeTable", source.Uuid),
		bytes.NewBufferString(fmt.Sprintf(`{"newTableID":%d}`, destinationTableID)),
	)
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatalf("move table request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != fiber.StatusConflict {
		t.Fatalf("expected 409, got %d", response.StatusCode)
	}

	var refreshedSource model.Visitation
	if err := db.Db.First(&refreshedSource, source.ID).Error; err != nil {
		t.Fatalf("reload source visitation: %v", err)
	}
	if refreshedSource.TableID != sourceTableID {
		t.Fatalf("source table moved unexpectedly: got %d want %d", refreshedSource.TableID, sourceTableID)
	}
}

func TestChangeTableRejectsInactiveDestination(t *testing.T) {
	userID, sourceTableID, destinationTableID := setupPOSTestDB(t)
	app := newPOSTestApp(userID)

	if status, err := postOpenTable(app, sourceTableID); err != nil || status != fiber.StatusOK {
		t.Fatalf("open source table: status=%d err=%v", status, err)
	}
	if err := db.Db.Model(&model.SettingTable{}).
		Where("id = ?", destinationTableID).
		Update("is_active", 0).Error; err != nil {
		t.Fatalf("deactivate destination table: %v", err)
	}

	var source model.Visitation
	if err := db.Db.Where("table_id = ? AND is_active = 1", sourceTableID).First(&source).Error; err != nil {
		t.Fatalf("find source visitation: %v", err)
	}

	request := httptest.NewRequest(
		"PUT",
		fmt.Sprintf("/point-of-sales/%s/visitation/changeTable", source.Uuid),
		bytes.NewBufferString(fmt.Sprintf(`{"newTableID":%d}`, destinationTableID)),
	)
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatalf("move table request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != fiber.StatusNotFound {
		t.Fatalf("expected 404, got %d", response.StatusCode)
	}

	var refreshedSource model.Visitation
	if err := db.Db.First(&refreshedSource, source.ID).Error; err != nil {
		t.Fatalf("reload source visitation: %v", err)
	}
	if refreshedSource.TableID != sourceTableID {
		t.Fatalf("source table moved unexpectedly: got %d want %d", refreshedSource.TableID, sourceTableID)
	}
}

func TestEventsStreamsInitialSync(t *testing.T) {
	userID, _, _ := setupPOSTestDB(t)
	app := newPOSTestApp(userID)
	app.Get("/point-of-sales/events", Events)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	go func() {
		_ = app.Listener(listener)
	}()
	defer app.ShutdownWithTimeout(500 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+listener.Addr().String()+"/point-of-sales/events", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("open event stream: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", response.StatusCode)
	}
	if contentType := response.Header.Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("unexpected content type: %s", contentType)
	}

	reader := bufio.NewReader(response.Body)
	eventLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read event type: %v", err)
	}
	if strings.TrimSpace(eventLine) != "event: pos-sync" {
		t.Fatalf("unexpected event type: %q", eventLine)
	}
}

func TestOrderStoreTreatsDuplicateQuantityAsSuccess(t *testing.T) {
	userID, tableID, _ := setupPOSTestDB(t)
	app := newPOSTestApp(userID)

	if status, err := postOpenTable(app, tableID); err != nil || status != fiber.StatusOK {
		t.Fatalf("open table: status=%d err=%v", status, err)
	}

	var visitation model.Visitation
	if err := db.Db.Where("table_id = ? AND is_active = 1", tableID).First(&visitation).Error; err != nil {
		t.Fatalf("find visitation: %v", err)
	}

	product := model.Product{Name: "Water", Unit: "bottle", Price: 10, IsActive: true}
	if err := db.Db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}

	body := fmt.Sprintf(`{"product_id":%d,"quantity":1,"price":10}`, product.ID)
	for index := 0; index < 2; index++ {
		request := httptest.NewRequest(
			"POST",
			fmt.Sprintf("/point-of-sales/%s/visitation/order/store", visitation.Uuid),
			bytes.NewBufferString(body),
		)
		request.Header.Set("Content-Type", "application/json")
		response, err := app.Test(request, -1)
		if err != nil {
			t.Fatalf("order request %d: %v", index+1, err)
		}
		defer response.Body.Close()
		if response.StatusCode != fiber.StatusOK {
			t.Fatalf("request %d expected 200, got %d", index+1, response.StatusCode)
		}
	}

	var service model.Service
	if err := db.Db.Where("visitation_id = ? AND product_id = ?", visitation.ID, product.ID).First(&service).Error; err != nil {
		t.Fatalf("find service: %v", err)
	}
	if service.SellQuantity != 1 {
		t.Fatalf("duplicate request changed quantity: got %.2f want 1", service.SellQuantity)
	}
}

func TestOrderStoreIncrementAddsEachClick(t *testing.T) {
	userID, tableID, _ := setupPOSTestDB(t)
	app := newPOSTestApp(userID)

	if status, err := postOpenTable(app, tableID); err != nil || status != fiber.StatusOK {
		t.Fatalf("open table: status=%d err=%v", status, err)
	}

	var visitation model.Visitation
	if err := db.Db.Where("table_id = ? AND is_active = 1", tableID).First(&visitation).Error; err != nil {
		t.Fatalf("find visitation: %v", err)
	}

	product := model.Product{Name: "Soda", Unit: "bottle", Price: 15, IsActive: true}
	if err := db.Db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}

	body := fmt.Sprintf(`{"product_id":%d,"quantity":1,"price":15,"action":"increment"}`, product.ID)
	for index := 0; index < 2; index++ {
		request := httptest.NewRequest(
			"POST",
			fmt.Sprintf("/point-of-sales/%s/visitation/order/store", visitation.Uuid),
			bytes.NewBufferString(body),
		)
		request.Header.Set("Content-Type", "application/json")
		response, err := app.Test(request, -1)
		if err != nil {
			t.Fatalf("increment request %d: %v", index+1, err)
		}
		defer response.Body.Close()
		if response.StatusCode != fiber.StatusOK {
			t.Fatalf("increment request %d expected 200, got %d", index+1, response.StatusCode)
		}
	}

	var service model.Service
	if err := db.Db.Where("visitation_id = ? AND product_id = ?", visitation.ID, product.ID).First(&service).Error; err != nil {
		t.Fatalf("find service: %v", err)
	}
	if service.SellQuantity != 2 {
		t.Fatalf("increment did not add each click: got %.2f want 2", service.SellQuantity)
	}
	if service.TotalCost != 30 {
		t.Fatalf("total cost: got %.2f want 30", service.TotalCost)
	}
}

func TestCalculateGameFeeKeepsPracticeAtFullFirstHour(t *testing.T) {
	tests := []struct {
		name         string
		seconds      int64
		isDiscounted bool
		want         float64
	}{
		{name: "regular under 30 minutes charges half hour", seconds: 29 * 60, isDiscounted: false, want: 50},
		{name: "regular over 30 minutes charges full hour", seconds: 31 * 60, isDiscounted: false, want: 100},
		{name: "practice under 30 minutes charges full practice hour", seconds: 10 * 60, isDiscounted: true, want: 80},
		{name: "practice at 60 minutes charges full practice hour", seconds: 60 * 60, isDiscounted: true, want: 80},
		{name: "practice over 60 minutes prorates after first hour", seconds: 75 * 60, isDiscounted: true, want: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateGameFee(tt.seconds, 100, 80, tt.isDiscounted)
			if math.Abs(got-tt.want) > 0.0001 {
				t.Fatalf("CalculateGameFee() = %v, want %v", got, tt.want)
			}
		})
	}
}
