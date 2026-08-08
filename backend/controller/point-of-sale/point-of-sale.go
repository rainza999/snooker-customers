package pointofsale

import (
	"bufio"
	"encoding/json"
	"errors" // สำหรับการใช้งาน errors.Is
	"fmt"
	"log"
	"math"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
	"github.com/rainza999/fiber-test/realtime"
	"github.com/rainza999/fiber-test/relay"
	"github.com/valyala/fasthttp"
	"golang.org/x/crypto/bcrypt"

	"gorm.io/gorm" // สำหรับการใช้ gorm.ErrRecordNotFound
)

var (
	errTableAlreadyActive = errors.New("table is already active")
	errTableNotFound      = errors.New("table not found")
	errTablePriceMismatch = errors.New("table price does not match")
	posMutationMu         sync.Mutex
)

func currentUserDivisionID(c *fiber.Ctx) (uint, error) {
	userID, ok := c.Locals("userID").(int)
	if !ok {
		return 0, errors.New("missing user context")
	}

	var user model.User
	if err := db.Db.First(&user, userID).Error; err != nil {
		return 0, err
	}

	return user.DivisionID, nil
}

func broadcastPOSUpdate(divisionID uint, action string) {
	realtime.PublishPOS(divisionID, action)
}

func writePOSEvent(writer *bufio.Writer, event realtime.POSEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", event.Type, payload); err != nil {
		return err
	}

	return writer.Flush()
}

func Events(c *fiber.Ctx) error {
	divisionID, err := currentUserDivisionID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"authenticated": false,
			"message":       "Unauthorized: Missing user context",
		})
	}

	events, unsubscribe := realtime.SubscribePOS(divisionID)

	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache, no-store, must-revalidate")
	c.Set(fiber.HeaderConnection, "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(writer *bufio.Writer) {
		defer unsubscribe()

		if err := writePOSEvent(writer, realtime.NewPOSEvent("sync")); err != nil {
			return
		}

		heartbeat := time.NewTicker(20 * time.Second)
		defer heartbeat.Stop()

		for {
			select {
			case event, ok := <-events:
				if !ok {
					return
				}
				if err := writePOSEvent(writer, event); err != nil {
					return
				}
			case <-heartbeat.C:
				if _, err := writer.WriteString(": keep-alive\n\n"); err != nil {
					return
				}
				if err := writer.Flush(); err != nil {
					return
				}
			}
		}
	}))

	return nil
}

func AnyData(c *fiber.Ctx) error {

	userID, ok := c.Locals("userID").(int)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"authenticated": false,
			"message":       "Unauthorized: Missing user context",
		})
	}

	var user model.User
	if err := db.Db.First(&user, int(userID)).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"authenticated": false,
			"message":       "User not found",
		})
	}

	// Get DivisionID from the user
	divisionID := user.DivisionID
	log.Println("DivisionID:", divisionID)

	type ServiceData struct {
		ID           int     `json:"id"`
		VisitationID int     `json:"visitation_id"`
		ProductID    int     `json:"product_id"`
		ProductName  string  `json:"product_name"`
		SellQuantity float64 `json:"sell_quantity"`
		TotalCost    float64 `json:"total_cost"`
	}

	var tables []struct {
		model.SettingTable
		Status         string    `json:"status"`
		StartTime      time.Time `json:"start_time"`
		ResumeTime     time.Time `json:"resume_time"`
		UseTime        int64     `json:"use_time"`
		PauseTime      time.Time `json:"pause_time"`
		PausedDuration int64     `json:"paused_duration"`
		IsRunning      uint8     `json:"is_running"`
		UUID           string    `json:"uuid"`
		VisitationID   int       `json:"visitation_id"`
		// Services       []ServiceData `json:"services"` // Services ที่จะรวมเข้าไป
		Services []ServiceData `json:"services" gorm:"foreignKey:VisitationID"`
	}

	// ดึงข้อมูลโต๊ะและ visitation
	result := db.Db.Raw(`
        SELECT
            st.*,
            CASE
                WHEN v.id IS NOT NULL AND (v.is_paid = 0 OR v.is_paid = 2) AND v.is_active = 1 AND v.deleted_at IS NULL THEN 'open'
                ELSE 'closed'
            END as status,
            v.start_time,
            v.use_time,
			v.resume_time,
            v.paused_duration,
			v.pause_time,
			v.is_running,
            v.uuid,
			v.id as visitation_id
        FROM setting_tables st
        LEFT JOIN visitations v ON st.id = v.table_id AND v.is_active = 1 AND (v.is_paid = 0 OR v.is_paid = 2) AND v.deleted_at IS NULL
		 WHERE st.division_id = ? and v.deleted_at is null
		 ORDER BY CASE WHEN st.sort_order IS NULL OR st.sort_order = 0 THEN st.id ELSE st.sort_order END ASC, st.id ASC
    
    `, divisionID).Scan(&tables)

	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	}

	// ดึงข้อมูล services สำหรับทุก visitation
	var services []ServiceData
	serviceResult := db.Db.Raw(`
		SELECT
			s.id,
			s.visitation_id,
			s.product_id,
			p.name as product_name,
			s.sell_quantity,
			s.total_cost
		FROM services s
		LEFT JOIN products p ON s.product_id = p.id
		WHERE s.deleted_at IS NULL
	`).Scan(&services)

	if serviceResult.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": serviceResult.Error.Error()})
	}

	// สร้างแผนที่สำหรับเก็บ services ตาม visitation_id
	serviceMap := make(map[int][]ServiceData)
	for _, service := range services {
		serviceMap[service.VisitationID] = append(serviceMap[service.VisitationID], service)
	}

	// รวมข้อมูล services เข้าไปในแต่ละ table ที่มี visitation
	for i := range tables {
		tables[i].Services = serviceMap[tables[i].VisitationID]
	}

	// ส่งข้อมูลโต๊ะพร้อม services กลับไปยัง frontend
	return c.JSON(tables)
}

type StoreBody struct {
	TableID uint   `json:"tableID"`
	Status  string `json:"status"`
}

func publishTableRelayByID(tableID uint, on bool, action string) fiber.Map {
	var table model.SettingTable
	if err := db.Db.First(&table, tableID).Error; err != nil {
		log.Printf("relay %s skipped: table_id=%d not found: %v", action, tableID, err)
		return fiber.Map{
			"ok":      false,
			"skipped": true,
			"action":  action,
			"error":   "table not found",
		}
	}

	return publishTableRelay(table, on, action)
}

func publishTableRelay(table model.SettingTable, on bool, action string) fiber.Map {
	state := "off"
	if on {
		state = "on"
	}

	result := fiber.Map{
		"enabled": relay.Enabled(),
		"relay":   table.Relay,
		"state":   state,
		"action":  action,
	}

	if table.Relay == 0 {
		result["ok"] = true
		result["skipped"] = true
		result["reason"] = "no relay mapped"
		return result
	}

	if !relay.Enabled() {
		result["ok"] = true
		result["skipped"] = true
		result["reason"] = "mqtt relay not configured"
		return result
	}

	if err := relay.SetRelay(table.Relay, on); err != nil {
		log.Printf("relay %s failed: table_id=%d relay=%d state=%s error=%v", action, table.ID, table.Relay, state, err)
		result["ok"] = false
		result["error"] = "relay command failed"
		return result
	}

	result["ok"] = true
	return result
}

func publishPaidVisitationRelay(visitation model.Visitation, isPaid uint8) fiber.Map {
	if isPaid != 1 {
		return fiber.Map{
			"ok":      true,
			"skipped": true,
			"reason":  "visitation is not paid",
		}
	}

	return publishTableRelayByID(visitation.TableID, false, "payment")
}

func Store(c *fiber.Ctx) error {
	var json StoreBody
	body := c.Body()
	fmt.Println("Raw request body:", string(body))

	if err := c.BodyParser(&json); err != nil {
		fmt.Println("Error parsing body:", err)
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if json.Status == "open" {
		divisionID, err := currentUserDivisionID(c)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized: Missing user context"})
		}

		posMutationMu.Lock()
		defer posMutationMu.Unlock()

		now := time.Now()
		var visitation model.Visitation
		var settingTable model.SettingTable
		err = db.Db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("id = ? AND division_id = ? AND is_active = 1", json.TableID, divisionID).
				First(&settingTable).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errTableNotFound
				}
				return err
			}

			var activeCount int64
			if err := tx.Model(&model.Visitation{}).
				Where("table_id = ? AND is_active = 1 AND (is_paid = 0 OR is_paid = 2) AND deleted_at IS NULL", json.TableID).
				Count(&activeCount).Error; err != nil {
				return err
			}
			if activeCount > 0 {
				return errTableAlreadyActive
			}

			visitation = model.Visitation{
				TableID:    json.TableID,
				DivisionID: divisionID,
				VisitDate:  now.Truncate(24 * time.Hour),
				StartTime:  now,
				ResumeTime: now,
				UseTime:    0,
				IsRunning:  1,
				PauseTime:  now,
				TotalCost:  0,
				NetPrice:   0,
				IsPaid:     0,
				IsVisit:    0,
				IsActive:   1,
			}
			if err := tx.Create(&visitation).Error; err != nil {
				return err
			}

			var product model.Product
			if err := tx.Where("is_snooker_time = ?", true).First(&product).Error; err != nil {
				return err
			}

			service := model.Service{
				VisitationID:   visitation.ID,
				ProductID:      product.ID,
				SellQuantity:   1,
				TotalFIFO_Cost: settingTable.Price,
				TotalCost:      settingTable.Price,
				NetPrice:       settingTable.Price,
				UseTime:        0,
				Status:         "draft",
			}

			return tx.Create(&service).Error
		})
		if errors.Is(err, errTableAlreadyActive) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Table is already active"})
		}
		if errors.Is(err, errTableNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Table not found"})
		}
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to open table"})
		}

		relayResult := publishTableRelay(settingTable, true, "open")

		var table []struct {
			model.SettingTable
			Status         string    `json:"status"`
			StartTime      time.Time `json:"start_time"`
			ResumeTime     time.Time `json:"resume_time"`
			UseTime        int64     `json:"use_time"`
			PauseTime      time.Time `json:"pause_time"`
			PausedDuration int64     `json:"paused_duration"`
			IsRunning      uint8     `json:"is_running"`
			UUID           string    `json:"uuid"`
			VisitationID   int       `json:"visitation_id"`
			// Services       []ServiceData `json:"services" gorm:"foreignKey:VisitationID"`
		}

		// ดึงข้อมูลโต๊ะและ visitation
		result2 := db.Db.Raw(`
    SELECT
        st.*,
        CASE
            WHEN v.id IS NOT NULL AND (v.is_paid = 0 OR v.is_paid = 2) AND v.is_active = 1 AND v.deleted_at IS NULL THEN 'open'
            ELSE 'closed'
        END as status,
        v.start_time,
		v.resume_time,
        v.use_time,
        v.paused_duration,
        v.pause_time,
		v.is_running,
        v.uuid,
        v.id as visitation_id
    FROM setting_tables st
    LEFT JOIN visitations v ON st.id = v.table_id AND v.is_active = 1 AND (v.is_paid = 0 OR v.is_paid = 2) AND v.deleted_at IS NULL
     WHERE v.uuid = ?

`, visitation.Uuid).Scan(&table)

		if result2.Error != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result2.Error.Error()})
		}
		broadcastPOSUpdate(divisionID, "table-opened")

		return c.JSON(fiber.Map{
			"message":    "Data processed successfully",
			"uuid":       visitation.Uuid,
			"code":       visitation.Code,
			"start_time": visitation.StartTime,
			"table":      table,
			"relay":      relayResult,
		})
	} else if json.Status == "close" {
		divisionID, err := currentUserDivisionID(c)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized: Missing user context"})
		}

		posMutationMu.Lock()
		defer posMutationMu.Unlock()

		// ปิดโต๊ะที่มีอยู่โดยตั้งค่า is_active = 0
		result := db.Db.Model(&model.Visitation{}).
			Where("table_id = ? AND division_id = ? AND is_active = 1", json.TableID, divisionID).
			Updates(map[string]interface{}{
				"is_active":  0,
				"is_running": 0,
				"end_time":   time.Now(),
			})

		if result.Error != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
		}
		if result.RowsAffected == 0 {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Table is not active"})
		}
		relayResult := publishTableRelayByID(json.TableID, false, "close")
		broadcastPOSUpdate(divisionID, "table-closed")

		return c.JSON(fiber.Map{
			"message": "Table closed successfully",
			"relay":   relayResult,
		})
	}

	return c.JSON(fiber.Map{"message": "Invalid status"})
}

// type UpdateUseTimeBody struct {
// 	UUID string `json:"uuid"`
// }

// func UpdateUseTime(c *fiber.Ctx) error {
// 	var json UpdateUseTimeBody

// 	if err := c.BodyParser(&json); err != nil {
// 		return c.Status(400).JSON(fiber.Map{
// 			"error": "Invalid request body",
// 		})
// 	}

// 	var visitation model.Visitation
// 	if err := db.Db.Where("uuid = ?", json.UUID).First(&visitation).Error; err != nil {
// 		return c.Status(404).JSON(fiber.Map{"error": "Record not found"})
// 	}

// 	visitation.UseTime = visitation.UseTime.Add(time.Minute)
// 	if err := db.Db.Save(&visitation).Error; err != nil {
// 		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
// 	}

// 	return c.JSON(fiber.Map{"message": "UseTime updated successfully"})
// }

type ServiceData struct {
	ProductID    int     `json:"product_id"`
	SellQuantity float64 `json:"sell_quantity"`
	TotalCost    string  `json:"total_cost"`
	NetPrice     string  `json:"net_price"`
}

type PaymentData struct {
	UUID       string        `json:"uuid"`
	TotalCost  string        `json:"total_cost"`
	NetPrice   string        `json:"net_price"`
	IsPaid     uint8         `json:"is_paid"`
	EndTime    string        `json:"end_time"`
	PaidAmount string        `json:"paid_amount"`
	TableType  uint8         `json:"table_type"`
	Services   []ServiceData `json:"services"` // เพิ่มฟิลด์ services
}

func legacyPaymentStore(c *fiber.Ctx) error {
	// อ่านข้อมูลจาก request body
	var paymentData PaymentData
	// if err := c.BodyParser(&paymentData); err != nil {
	// 	return c.Status(400).JSON(fiber.Map{
	// 		"error": "Invalid request body",
	// 	})
	// }

	if err := c.BodyParser(&paymentData); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid request body",
			"details": err.Error(),
		})
	}

	// ตรวจสอบฟิลด์ที่จำเป็น
	if paymentData.UUID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid field",
			"details": "UUID is required",
		})
	}

	if paymentData.TotalCost == "" {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid field",
			"details": "TotalCost is required",
		})
	}

	if paymentData.NetPrice == "" {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid field",
			"details": "NetPrice is required",
		})
	}

	if paymentData.PaidAmount == "" {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid field",
			"details": "PaidAmount is required",
		})
	}

	if paymentData.EndTime == "" {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid field",
			"details": "EndTime is required",
		})
	}

	if paymentData.TableType != 0 && paymentData.TableType != 1 {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid field",
			"details": "TableType must be 0 or 1",
		})
	}

	// ตรวจสอบฟิลด์ Services
	for i, service := range paymentData.Services {
		if service.ProductID == 0 {
			return c.Status(400).JSON(fiber.Map{
				"error":   "Invalid field",
				"details": fmt.Sprintf("Service[%d]: ProductID is required", i),
			})
		}
		if service.SellQuantity == 0 {
			return c.Status(400).JSON(fiber.Map{
				"error":   "Invalid field",
				"details": fmt.Sprintf("Service[%d]: SellQuantity is required", i),
			})
		}
		if service.TotalCost == "" {
			return c.Status(400).JSON(fiber.Map{
				"error":   "Invalid field",
				"details": fmt.Sprintf("Service[%d]: TotalCost is required", i),
			})
		}
		if service.NetPrice == "" {
			return c.Status(400).JSON(fiber.Map{
				"error":   "Invalid field",
				"details": fmt.Sprintf("Service[%d]: NetPrice is required", i),
			})
		}
	}

	// ค้นหา visitation ตาม UUID ที่ได้รับมา
	var visitation model.Visitation
	if err := db.Db.Where("uuid = ?", paymentData.UUID).First(&visitation).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Record not found"})
	}

	// ✅ ถ้าโต๊ะหยุด (is_running == 0) ให้ resume
	if visitation.IsRunning == 0 {
		if !visitation.PauseTime.IsZero() {
			resumeTime := time.Now()
			pausedGap := resumeTime.Sub(visitation.PauseTime).Seconds()
			visitation.PausedDuration += int64(pausedGap)
			visitation.IsRunning = 1
			visitation.ResumeTime = resumeTime
			visitation.PauseTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC) // reset pause_time
		}
	}

	// ✅ คำนวณเวลาการใช้งานจริง (ไม่ปัด)
	var elapsedTime float64
	startTime := visitation.StartTime
	now := time.Now()

	if !startTime.IsZero() {
		elapsedTime = now.Sub(startTime).Seconds()

		// ถ้ามี paused_duration ให้หักออก
		if visitation.PausedDuration > 0 {
			elapsedTime -= float64(visitation.PausedDuration)
		}

		// ถ้ายัง pause อยู่ ใช้เวลาถึง pause_time
		if !visitation.PauseTime.IsZero() && visitation.PauseTime.Year() != 2000 {
			elapsedTime = visitation.PauseTime.Sub(startTime).Seconds() - float64(visitation.PausedDuration)
		}
	}

	if elapsedTime < 0 {
		elapsedTime = 0
	}

	// ✅ เก็บ use_time เป็นวินาที (ตามจริง)
	visitation.UseTime = int64(elapsedTime)

	// ตรวจสอบว่า visitation มี BillCode หรือยัง
	if visitation.BillCode == "" {
		// ค้นหา Division เพื่อนำ Code มาใช้
		var division model.Division
		if err := db.Db.Where("id = ?", visitation.DivisionID).First(&division).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Division not found"})
		}

		// สร้าง BillCode โดยใช้รูปแบบ XXYYMMDDXXX
		currentDate := time.Now().Format("060102") // YYMMDD
		latestVisitation := model.Visitation{}

		// ค้นหาหมายเลขบิลล่าสุดของวันนี้จากตาราง visitation
		err := db.Db.Where("bill_code LIKE ?", division.Code+currentDate+"%").
			Order("bill_code DESC").First(&latestVisitation).Error

		var newBillNumber int
		if err == nil {
			// ถ้ามีบิลอยู่แล้วให้เพิ่มเลขบิล
			latestBillCode := latestVisitation.BillCode[len(latestVisitation.BillCode)-3:]
			latestBillNumber, _ := strconv.Atoi(latestBillCode)
			newBillNumber = latestBillNumber + 1
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			// ถ้ายังไม่มีบิลให้เริ่มจาก 001
			newBillNumber = 1
		} else {
			return c.Status(500).JSON(fiber.Map{"error": "Error retrieving latest bill"})
		}

		// ฟอร์แมต XXX เป็นเลข 3 หลัก
		billCode := fmt.Sprintf("%s%s%03d", division.Code, currentDate, newBillNumber)

		// อัปเดต BillCode ให้กับ visitation
		visitation.BillCode = billCode
	}

	// แปลงข้อมูลจาก string เป็น float64
	totalCost, err := strconv.ParseFloat(paymentData.TotalCost, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid total_cost format"})
	}

	netPrice, err := strconv.ParseFloat(paymentData.NetPrice, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid net_price format"})
	}

	paidAmount, err := strconv.ParseFloat(paymentData.PaidAmount, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid paid_amount format"})
	}

	changeAmount := paidAmount - netPrice
	if changeAmount < 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Paid amount is less than net price"})
	}

	// อัปเดตข้อมูล visitation
	endTime, err := time.Parse(time.RFC3339, paymentData.EndTime)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid end_time format"})
	}

	// โหลดเขตเวลา Asia/Bangkok
	location, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to load timezone"})
	}

	// แปลงเวลาจาก UTC ไปเป็น Asia/Bangkok
	endTimeInBangkok := endTime.In(location)

	visitation.TotalCost = totalCost
	visitation.NetPrice = netPrice
	visitation.PaidAmount = paidAmount
	visitation.ChangeAmount = changeAmount
	visitation.IsPaid = paymentData.IsPaid
	visitation.TableType = uint(paymentData.TableType)
	visitation.EndTime = endTimeInBangkok

	// บันทึกการเปลี่ยนแปลงลงในฐานข้อมูล
	if err := db.Db.Save(&visitation).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// อัปเดตหรือเพิ่ม service ที่เกี่ยวข้อง
	if err := db.Db.Model(&model.Service{}).
		Where("visitation_id = ? AND product_id = ?", visitation.ID, 1).
		Updates(map[string]interface{}{
			"use_time": visitation.UseTime,
			"status":   "paid",
		}).Error; err != nil {
		log.Printf("⚠️ Failed to update service (product_id=1): %v", err)
	}
	for _, serviceData := range paymentData.Services {
		var service model.Service
		// ตรวจสอบว่ามี service สำหรับ product_id นั้นๆ อยู่หรือไม่
		if err := db.Db.Where("visitation_id = ? AND product_id = ?", visitation.ID, serviceData.ProductID).First(&service).Error; err == nil {
			// ถ้ามีอยู่แล้วให้ทำการอัปเดต
			sellQuantity := serviceData.SellQuantity
			totalCost, err := strconv.ParseFloat(serviceData.TotalCost, 64)
			if err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "Invalid total_cost format"})
			}

			netPrice, err := strconv.ParseFloat(serviceData.NetPrice, 64)
			if err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "Invalid net_price format"})
			}

			service.SellQuantity = sellQuantity
			service.TotalCost = totalCost
			service.NetPrice = netPrice
			service.Status = "paid"

			if err := db.Db.Save(&service).Error; err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Failed to update service"})
			}
		} else {
			// ถ้ายังไม่มี service สำหรับ product_id นั้นให้สร้างใหม่
			totalCost, err := strconv.ParseFloat(serviceData.TotalCost, 64)
			if err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "Invalid total_cost format"})
			}

			netPrice, err := strconv.ParseFloat(serviceData.NetPrice, 64)
			if err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "Invalid net_price format"})
			}

			newService := model.Service{
				VisitationID: visitation.ID,
				ProductID:    uint(serviceData.ProductID),
				SellQuantity: serviceData.SellQuantity,
				TotalCost:    totalCost,
				NetPrice:     netPrice,
				Status:       "paid",
			}
			if err := db.Db.Create(&newService).Error; err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Failed to create new service"})
			}
		}
	}

	broadcastPOSUpdate(visitation.DivisionID, "payment-updated")

	// ส่ง response กลับไปยัง frontend
	return c.JSON(fiber.Map{
		"relay":     publishPaidVisitationRelay(visitation, paymentData.IsPaid),
		"message":   "PaymentStore updated successfully",
		"bill_code": visitation.BillCode, // ส่งเลขที่ BillCode กลับไปด้วย
	})
}

func Live(c *fiber.Ctx) error {
	// uuid := c.Params("uuid")

	// var visitation model.Visitation
	// if err := db.Db.Where("uuid = ?", uuid).First(&visitation).Error; err != nil {
	// 	return c.Status(404).JSON(fiber.Map{"error": "Record not found"})
	// }

	cmd := exec.Command("C:\\Program Files\\obs-studio\\bin\\64bit\\obs64.exe")
	cmd.Dir = "C:\\Program Files\\obs-studio\\bin\\64bit" // 💡 สำคัญ!
	return cmd.Start()

	// return c.JSON(visitation);
}

func GetVisitationByUUID(c *fiber.Ctx) error {
	uuid := c.Params("uuid")

	// ค้นหา visitation ตาม UUID ที่ได้รับมา
	var visitation model.Visitation
	if err := db.Db.Where("uuid = ?", uuid).First(&visitation).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Record not found"})
	}

	// ส่งข้อมูล visitation กลับไปยัง frontend
	return c.JSON(visitation)
}

// func UpdatePausedDurationTime(c *fiber.Ctx) error {
// 	fmt.Println(string(c.Body())) // log body สำหรับ debug

// 	var request struct {
// 		UUID           string `json:"uuid"`
// 		Action         string `json:"action"` // "pause" หรือ "resume"
// 		PausedDuration int64  `json:"pausedDuration"`
// 		PauseTime      string `json:"pauseTime"`  // สำหรับ pause
// 		ResumeTime     string `json:"resumeTime"` // สำหรับ resume
// 		UseTime        int64  `json:"useTime"`    // เวลาที่เล่นจริง (วินาที)
// 	}

// 	if err := c.BodyParser(&request); err != nil {
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
// 			"error": "Cannot parse JSON",
// 		})
// 	}

// 	var visitation model.Visitation
// 	if err := db.Db.Where("uuid = ?", request.UUID).First(&visitation).Error; err != nil {
// 		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
// 			"error": "Visitation not found",
// 		})
// 	}

// 	// แยกกรณีตาม Action
// 	switch request.Action {
// 	case "pause":
// 		// 🛑 กรณี Pause
// 		pauseTime, _ := time.Parse(time.RFC3339, request.PauseTime)

// 		visitation.IsRunning = 0
// 		visitation.PauseTime = pauseTime
// 		// visitation.UseTime = time.Date(2000, 1, 1, 0, 0, int(request.UseTime), 0, time.UTC)
// 		// visitation.PausedDuration = // ใช้เวลาที่เล่นจริง (วินาที)

// 	case "resume":
// 		// if visitation.PausedDuration == 0 {
// 		visitation.IsRunning = 1
// 		visitation.PausedDuration = visitation.PausedDuration + time.Now().Unix() - visitation.PauseTime.Unix()
// 		visitation.PauseTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
// 		// }
// 		// resumeTime, _ := time.Parse(time.RFC3339, request.ResumeTime)

// 		/**
// 				visitation.PausedDuration = time.Now().Unix() - visitation.PauseTime.Unix()
// 		visitation.PauseTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
// 		*/
// 		// if !visitation.PauseTime.IsZero() {
// 		// 	pausedGap := resumeTime.Sub(visitation.PauseTime).Seconds()
// 		// 	visitation.PausedDuration += int64(pausedGap)
// 		// }

// 		// visitation.ResumeTime = resumeTime
// 		// visitation.IsRunning = 1

// 	default:
// 		// ถ้าไม่ได้ส่ง action ให้ใช้ fallback เดิม (รองรับเวอร์ชันเก่า)
// 		if request.PausedDuration == 0 {
// 			visitation.IsRunning = 1
// 			visitation.PauseTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
// 			visitation.PausedDuration = 0
// 		} else {
// 			visitation.IsRunning = 0
// 			visitation.PausedDuration = request.PausedDuration
// 			visitation.PauseTime = time.Now()
// 		}
// 	}

// 	if err := db.Db.Save(&visitation).Error; err != nil {
// 		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
// 			"error": "Failed to update visitation",
// 		})
// 	}

// 	return c.JSON(fiber.Map{
// 		"message": fmt.Sprintf("Visitation %s updated successfully", request.Action),
// 	})
// }

func UpdatePausedDurationTime(c *fiber.Ctx) error {
	fmt.Println(string(c.Body()))

	var request struct {
		UUID           string `json:"uuid"`
		Action         string `json:"action"`
		PausedDuration int64  `json:"pausedDuration"`
		PauseTime      string `json:"pauseTime"`
		ResumeTime     string `json:"resumeTime"`
		UseTime        int64  `json:"useTime"`
	}

	if err := c.BodyParser(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	var visitation model.Visitation
	if err := db.Db.Where("uuid = ?", request.UUID).First(&visitation).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Visitation not found",
		})
	}

	sentinelPauseTime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	switch request.Action {
	case "pause":
		pauseTime, err := time.Parse(time.RFC3339, request.PauseTime)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid pauseTime format",
			})
		}

		visitation.IsRunning = 0
		visitation.PauseTime = pauseTime

		// เก็บ use time จริงตอนกด pause/check bill
		if request.UseTime >= 0 {
			visitation.UseTime = request.UseTime
		}

		// อย่าไป reset pausedDuration ทิ้ง
		if request.PausedDuration >= 0 {
			visitation.PausedDuration = request.PausedDuration
		}

	case "resume":
		resumeTime, err := time.Parse(time.RFC3339, request.ResumeTime)
		if err != nil {
			resumeTime = time.Now()
		}

		// คิด paused duration เฉพาะตอนที่มัน pause อยู่จริงเท่านั้น
		if visitation.IsRunning == 0 &&
			!visitation.PauseTime.IsZero() &&
			visitation.PauseTime.After(sentinelPauseTime) {

			pausedGap := int64(resumeTime.Sub(visitation.PauseTime).Seconds())
			if pausedGap < 0 {
				pausedGap = 0
			}
			visitation.PausedDuration += pausedGap
		}

		visitation.IsRunning = 1
		visitation.PauseTime = sentinelPauseTime

	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid action",
		})
	}

	if err := db.Db.Save(&visitation).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update visitation",
		})
	}

	relayResult := fiber.Map{
		"ok":      true,
		"skipped": true,
		"reason":  "unsupported pause action",
	}
	if request.Action == "pause" {
		relayResult = publishTableRelayByID(visitation.TableID, false, "pause")
	} else if request.Action == "resume" {
		relayResult = publishTableRelayByID(visitation.TableID, true, "resume")
	}
	broadcastPOSUpdate(visitation.DivisionID, "table-"+request.Action)

	return c.JSON(fiber.Map{
		"message":         fmt.Sprintf("Visitation %s updated successfully", request.Action),
		"paused_duration": visitation.PausedDuration,
		"is_running":      visitation.IsRunning,
		"pause_time":      visitation.PauseTime,
		"use_time":        visitation.UseTime,
		"relay":           relayResult,
	})
}

// Struct สำหรับรับข้อมูลจาก request
type VerifyPasswordRequest struct {
	UUIDTable string `json:"uuidTable"` // ✅ ต้องเพิ่มอันนี้
	UUID      string `json:"uuid"`      // UUID ของ Employee
	Password  string `json:"password"`  // รหัสผ่านของ User
	TableID   uint   `json:"tableID"`
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
func VerifyPassword(c *fiber.Ctx) error {
	var request VerifyPasswordRequest

	// แปลง request body เป็น struct
	if err := c.BodyParser(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// ตรวจสอบว่า UUID และ Password ไม่เป็นค่าว่าง
	if request.UUID == "" || request.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "UUID and Password are required",
		})
	}

	// ดึงข้อมูล Employee โดยใช้ UUID
	var employee model.Employee
	if err := db.Db.Where("uuid = ?", request.UUID).First(&employee).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Employee not found",
		})
	}

	// ดึงข้อมูล User โดยใช้ user_id จาก Employee
	var user model.User
	if err := db.Db.Where("employee_id = ?", employee.ID).First(&user).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}
	// ตรวจสอบรหัสผ่านด้วย bcrypt
	if !CheckPasswordHash(request.Password, user.Password) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"message": "Invalid password",
		})
	}
	// if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(request.Password)); err != nil {
	// 	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
	// 		"success": false,
	// 		"message": "Invalid password",
	// 	})
	// }

	// ถ้ารหัสผ่านถูกต้อง ปิดโต๊ะที่มีอยู่โดยตั้งค่า is_active = 0
	posMutationMu.Lock()
	defer posMutationMu.Unlock()

	var visitation model.Visitation
	if err := db.Db.Where("table_id = ? AND is_active = 1", request.TableID).First(&visitation).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Active table not found"})
	}

	result := db.Db.Model(&model.Visitation{}).
		Where("id = ? AND is_active = 1", visitation.ID).
		Update("is_active", 0)

	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	}
	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Table is not active"})
	}
	relayResult := publishTableRelayByID(request.TableID, false, "verify-close")
	broadcastPOSUpdate(visitation.DivisionID, "table-closed")

	return c.JSON(fiber.Map{
		"message": "Table closed successfully",
		"relay":   relayResult,
	})
}

func VerifyPasswordAndCloseTable(c *fiber.Ctx) error {
	var request VerifyPasswordRequest

	// แปลง request body เป็น struct
	if err := c.BodyParser(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}
	if request.UUIDTable == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "UUID Table are required",
		})
	}
	// ตรวจสอบว่า UUID และ Password ไม่เป็นค่าว่าง
	if request.UUID == "" || request.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "UUID and Password are required",
		})
	}

	// ดึงข้อมูล Employee โดยใช้ UUID
	var employee model.Employee
	if err := db.Db.Where("uuid = ?", request.UUID).First(&employee).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Employee not found",
		})
	}

	// ดึงข้อมูล User โดยใช้ user_id จาก Employee
	var user model.User
	if err := db.Db.Where("employee_id = ?", employee.ID).First(&user).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}
	// ตรวจสอบรหัสผ่านด้วย bcrypt
	if !CheckPasswordHash(request.Password, user.Password) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"message": "Invalid password",
		})
	}

	posMutationMu.Lock()
	defer posMutationMu.Unlock()

	// ✅ ตรวจสอบว่ามีโต๊ะที่ยังเปิดอยู่หรือไม่ และเช็คค่า is_paid
	var visitation model.Visitation
	err := db.Db.Where("table_id = ? AND is_active = 1 and uuid = ?", request.TableID, request.UUIDTable).First(&visitation).Error
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Active table not found"})
	}

	if visitation.IsPaid == 1 {
		// ✅ โต๊ะถูกเช็คบิลแล้ว
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"message": "โต๊ะนี้มีการเช็คบิลแล้ว",
		})
	}

	// 4) ปิดเฉพาะแถวนี้เท่านั้น
	now := time.Now()

	// ✅ ถ้าโต๊ะหยุด (is_running == 0) ให้ resume
	if visitation.IsRunning == 0 {
		if !visitation.PauseTime.IsZero() {
			resumeTime := time.Now()
			pausedGap := resumeTime.Sub(visitation.PauseTime).Seconds()
			visitation.PausedDuration += int64(pausedGap)
			visitation.IsRunning = 1
			visitation.ResumeTime = resumeTime
			visitation.PauseTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC) // reset pause_time
		}
	}

	// ✅ คำนวณเวลาการใช้งานจริง (ไม่ปัด)
	var elapsedTime float64
	startTime := visitation.StartTime

	if !startTime.IsZero() {
		elapsedTime = now.Sub(startTime).Seconds()

		// ถ้ามี paused_duration ให้หักออก
		if visitation.PausedDuration > 0 {
			elapsedTime -= float64(visitation.PausedDuration)
		}

		// ถ้ายัง pause อยู่ ใช้เวลาถึง pause_time
		if !visitation.PauseTime.IsZero() && visitation.PauseTime.Year() != 2000 {
			elapsedTime = visitation.PauseTime.Sub(startTime).Seconds() - float64(visitation.PausedDuration)
		}
	}

	if elapsedTime < 0 {
		elapsedTime = 0
	}

	// ✅ เก็บ use_time เป็นวินาที (ตามจริง)
	visitation.UseTime = int64(elapsedTime)

	result := db.Db.Model(&model.Visitation{}).
		Where("table_id = ? AND uuid = ? AND is_active = 1", request.TableID, request.UUIDTable).
		Updates(map[string]interface{}{
			"is_active": 0,
			"end_time":  now, // ตั้งเวลาเลิก
			"use_time":  visitation.UseTime,
		})

		// 🟦 ดึงข้อมูล SettingTable เพื่อดูราคา
	var table model.SettingTable
	if err := db.Db.First(&table, request.TableID).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Table not found"})
	}

	var service model.Service
	result2 := db.Db.Where("visitation_id = ? AND product_id = 1 AND deleted_at IS NULL",
		visitation.ID).First(&service)

	if result2.Error != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Service not found"})
	}

	// 🟦 คำนวณราคาใหม่ตามเวลา
	isDiscounted := false
	fee := CalculateGameFee(visitation.UseTime, table.Price, table.Price2, isDiscounted)

	// 🟦 อัปเดตข้อมูลใน services
	db.Db.Model(&service).Updates(map[string]interface{}{
		"sell_quantity": float64(visitation.UseTime),
		"total_cost":    fee,
		"net_price":     fee,
	})
	// const calculateGameFee = (timeInSeconds, price, price2, isDiscounted) => {
	//     console.log("Calculating game fee with:", {
	//         timeInSeconds,
	//         price,
	//         price2,
	//         isDiscounted,
	//     });
	//     const ratePerHour = isDiscounted ? price2 : price;
	//     const ratePerMinute = ratePerHour / 60;

	//     const totalMinutes = Math.ceil(timeInSeconds / 60);

	//     let fee = 0;

	//     if (totalMinutes <= 30) {
	//         fee = ratePerMinute * 30; // คิดขั้นต่ำ 30 นาที
	//     } else if (totalMinutes <= 60) {
	//         fee = ratePerHour; // คิดเป็น 1 ชั่วโมง
	//     } else {
	//         const extraMinutes = totalMinutes - 60;
	//         fee = ratePerHour + extraMinutes * ratePerMinute; // คิดเกิน 1 ชั่วโมงเป็นนาที
	//     }

	//     fee = Math.round(fee * 100) / 100; // ปัดเป็น 2 ตำแหน่งทศนิยม
	//     return Math.ceil(fee); // ปัดขึ้นเป็นจำนวนเต็ม
	// };
	//อยากให้ update product_id = 1 ใน services ด้วย โดยที่คิด totalcost กับ netprice ใหม่ตาม use_time

	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	}
	if result.RowsAffected == 0 {
		// กันกรณี race condition: แถวถูกเปลี่ยนสภาพไปก่อนหน้า
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"message": "ไม่สามารถปิดโต๊ะได้ (อาจถูกปิดไปแล้ว)"})
	}

	relayResult := publishTableRelayByID(request.TableID, false, "verify-close-table")
	broadcastPOSUpdate(visitation.DivisionID, "table-closed")

	return c.JSON(fiber.Map{
		"message": "Table closed successfully",
		"relay":   relayResult,
	})
}

func PaymentPending(c *fiber.Ctx) error {
	uuid := c.Params("uuid")

	// ค้นหา visitation ตาม UUID ที่ได้รับมา
	var visitation model.Visitation
	if err := db.Db.Where("uuid = ?", uuid).First(&visitation).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Record not found"})
	}

	// ตรวจสอบว่า visitation มี BillCode หรือยัง
	if visitation.BillCode == "" {
		// ค้นหา Division เพื่อนำ Code มาใช้
		var division model.Division
		if err := db.Db.Where("id = ?", visitation.DivisionID).First(&division).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Division not found"})
		}

		// สร้าง BillCode โดยใช้รูปแบบ XXYYMMDDXXX
		currentDate := time.Now().Format("060102") // YYMMDD
		latestVisitation := model.Visitation{}

		// ค้นหาหมายเลขบิลล่าสุดของวันนี้จากตาราง visitation
		err := db.Db.Where("bill_code LIKE ?", division.Code+currentDate+"%").
			Order("bill_code DESC").First(&latestVisitation).Error

		var newBillNumber int
		if err == nil {
			// ถ้ามีบิลอยู่แล้วให้เพิ่มเลขบิล
			latestBillCode := latestVisitation.BillCode[len(latestVisitation.BillCode)-3:]
			latestBillNumber, _ := strconv.Atoi(latestBillCode)
			newBillNumber = latestBillNumber + 1
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			// ถ้ายังไม่มีบิลให้เริ่มจาก 001
			newBillNumber = 1
		} else {
			return c.Status(500).JSON(fiber.Map{"error": "Error retrieving latest bill"})
		}

		// ฟอร์แมต XXX เป็นเลข 3 หลัก
		billCode := fmt.Sprintf("%s%s%03d", division.Code, currentDate, newBillNumber)

		// อัปเดต BillCode ให้กับ visitation
		visitation.BillCode = billCode
		visitation.IsPaid = 2
	}

	// บันทึกการเปลี่ยนแปลงลงในฐานข้อมูล
	if err := db.Db.Save(&visitation).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	broadcastPOSUpdate(visitation.DivisionID, "payment-pending")

	// ส่ง response กลับไปยัง frontend
	return c.JSON(fiber.Map{
		"message":   "PaymentPending updated successfully",
		"bill_code": visitation.BillCode, // ส่งเลขที่ BillCode กลับไปด้วย
	})
}

// func OrderStore(c *fiber.Ctx) error {
// 	println("hello OrderStore")
// 	// ดึงข้อมูลจาก body request (ข้อมูลการสั่งซื้อ)
// 	uuid := c.Params("uuid")

// 	var order struct {
// 		VisitationID uint       `json:"visitation_id"` // ID ของการใช้โต๊ะ
// 		ProductID    uint       `json:"product_id"`    // ID ของสินค้า (เช่นอาหารหรือบริการ)
// 		Quantity     float64    `json:"quantity"`      // จำนวนที่สั่ง
// 		Price        float64    `json:"price"`         // ราคาต่อหน่วย
// 		Status       *string    `json:"status"`        // สถานะ (เช่น draft, delete)
// 		DeletedAt    *time.Time `json:"deleted_at"`    // เวลาที่ลบ (ถ้ามี)
// 	}

// 	// ตรวจสอบว่าการ parse ข้อมูลจาก body สำเร็จหรือไม่
// 	if err := c.BodyParser(&order); err != nil {
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
// 			"error": "Cannot parse order data",
// 		})
// 	}

// 	// ค้นหา visitation ตาม UUID ที่ได้รับมา
// 	var visitation model.Visitation
// 	if err := db.Db.Where("uuid = ?", uuid).First(&visitation).Error; err != nil {
// 		return c.Status(404).JSON(fiber.Map{"error": "Record not found"})
// 	}

// 	// ตรวจสอบว่า Product ที่สั่งมีอยู่ในระบบ
// 	var product model.Product
// 	if err := db.Db.First(&product, order.ProductID).Error; err != nil {
// 		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
// 			"error": "Product not found",
// 		})
// 	}

// 	// คำนวณราคาทั้งหมดจากจำนวนสินค้าที่สั่ง และราคาต่อหน่วย
// 	totalCost := order.Quantity * order.Price

// 	// ตรวจสอบว่ามี service สำหรับ Visitation นี้และสินค้านี้อยู่แล้วหรือไม่
// 	var existingService model.Service
// 	if err := db.Db.Where("visitation_id = ? AND product_id = ?", visitation.ID, product.ID).First(&existingService).Error; err == nil {
// 		// ถ้ามีสินค้าในรายการอยู่แล้ว ให้แทนค่าจำนวนและราคารวมใหม่ทั้งหมด
// 		existingService.SellQuantity = order.Quantity
// 		existingService.TotalCost = totalCost
// 		existingService.NetPrice = totalCost // สมมติว่าไม่มีส่วนลด

// 		// ตรวจสอบว่ามีการส่ง status หรือ deleted_at มาหรือไม่
// 		if order.Status != nil {
// 			existingService.Status = *order.Status
// 		}
// 		if order.DeletedAt != nil {
// 			existingService.DeletedAt = gorm.DeletedAt{Time: *order.DeletedAt, Valid: true} // แปลง time.Time ให้เป็น gorm.DeletedAt
// 		}

// 		if err := db.Db.Save(&existingService).Error; err != nil {
// 			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
// 				"error": "Failed to update existing service",
// 			})
// 		}

// 		return c.Status(fiber.StatusOK).JSON(fiber.Map{
// 			"message": "Order updated successfully",
// 			"service": existingService, // ส่งข้อมูล service ที่อัปเดตแล้วกลับไปให้ frontend
// 		})
// 	}

// 	// ถ้าไม่มีรายการนี้อยู่ใน service ให้สร้างใหม่
// 	service := model.Service{
// 		VisitationID: visitation.ID,      // เก็บ ID ของการใช้งานโต๊ะ
// 		ProductID:    product.ID,         // เก็บ ID ของสินค้าที่สั่ง
// 		SellQuantity: order.Quantity,     // เก็บจำนวนที่สั่ง
// 		TotalCost:    totalCost,          // เก็บราคารวม
// 		NetPrice:     totalCost,          // ยังไม่มีส่วนลด
// 		UseTime:      visitation.UseTime, // เก็บเวลาการใช้งาน (หากต้องการ)
// 		Status:       "draft",            // สถานะเริ่มต้นเป็น draft
// 	}

// 	// ตรวจสอบว่ามีการส่ง status หรือ deleted_at มาหรือไม่
// 	if order.Status != nil {
// 		service.Status = *order.Status
// 	}
// 	if order.DeletedAt != nil {
// 		service.DeletedAt = gorm.DeletedAt{Time: *order.DeletedAt, Valid: true}
// 	}

// 	// บันทึกข้อมูลลงในตาราง services
// 	if err := db.Db.Create(&service).Error; err != nil {
// 		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
// 			"error": "Failed to create service",
// 		})
// 	}

//		return c.Status(fiber.StatusOK).JSON(fiber.Map{
//			"message": "Order created successfully",
//			"service": service, // ส่งข้อมูล service กลับไปให้ frontend
//		})
//	}
func legacyOrderStore(c *fiber.Ctx) error {
	println("hello OrderStore")
	uuid := c.Params("uuid")

	var order struct {
		VisitationID uint       `json:"visitation_id"`
		ProductID    uint       `json:"product_id"`
		Quantity     float64    `json:"quantity"`
		Price        float64    `json:"price"`
		Status       *string    `json:"status"`
		DeletedAt    *time.Time `json:"deleted_at"`
	}

	if err := c.BodyParser(&order); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse order data",
		})
	}

	var visitation model.Visitation
	if err := db.Db.Where("uuid = ?", uuid).First(&visitation).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Record not found"})
	}

	var product model.Product
	if err := db.Db.First(&product, order.ProductID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Product not found",
		})
	}

	totalCost := order.Quantity * order.Price

	var existingService model.Service
	if err := db.Db.Where("visitation_id = ? AND product_id = ?", visitation.ID, product.ID).First(&existingService).Error; err == nil {
		addedQuantity := order.Quantity - existingService.SellQuantity

		if addedQuantity < 0 {
			// กรณีลดจำนวนที่ขายลง คำนวณจำนวนที่คืน stock
			removedQuantity := existingService.SellQuantity - order.Quantity

			// คืน stock ที่ถูกตัดออกกลับเข้าไปยัง stock_entries และคำนวณต้นทุนที่คืน
			totalReturnedCost, err := ReturnStockFIFO(order.ProductID, int(removedQuantity), db.Db)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "Failed to return stock",
				})
			}

			// อัปเดตจำนวนและต้นทุนใน service
			existingService.SellQuantity = order.Quantity
			existingService.TotalCost = order.Quantity * order.Price
			existingService.TotalFIFO_Cost -= totalReturnedCost // คืนต้นทุน FIFO ที่ถูกคืนออกไป
			existingService.NetPrice = order.Quantity * order.Price

			// ตรวจสอบว่าถ้า SellQuantity เป็น 0 เปลี่ยนสถานะเป็น "delete" และ stamp deleted_at
			if existingService.SellQuantity == 0 {
				existingService.Status = "delete"
				existingService.DeletedAt = gorm.DeletedAt{
					Time:  time.Now(),
					Valid: true,
				}
			}

			if err := db.Db.Save(&existingService).Error; err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "Failed to update service",
				})
			}
			broadcastPOSUpdate(visitation.DivisionID, "order-updated")

			return c.Status(fiber.StatusOK).JSON(fiber.Map{
				"message": "Order updated successfully, stock returned",
				"service": existingService,
			})
		} else if addedQuantity > 0 {
			// เพิ่มจำนวนขาย
			totalFIFO_Cost, err := CalculateFIFO(order.ProductID, int(addedQuantity), db.Db)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "Failed to calculate FIFO cost",
				})
			}

			existingService.SellQuantity = order.Quantity
			existingService.TotalCost = order.Quantity * order.Price
			existingService.TotalFIFO_Cost += totalFIFO_Cost
			existingService.NetPrice = order.Quantity * order.Price

			if order.Status != nil {
				existingService.Status = *order.Status
			}

			if order.DeletedAt != nil {
				existingService.DeletedAt = gorm.DeletedAt{Time: *order.DeletedAt, Valid: true}
			}

			if err := db.Db.Save(&existingService).Error; err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "Failed to update existing service",
				})
			}
			broadcastPOSUpdate(visitation.DivisionID, "order-updated")

			return c.Status(fiber.StatusOK).JSON(fiber.Map{
				"message": "Order updated successfully",
				"service": existingService,
			})
		}

		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "No changes made to the order",
		})
	}

	// กรณีเพิ่ม product ใหม่เข้ามาใน services
	totalFIFO_Cost, err := CalculateFIFO(order.ProductID, int(order.Quantity), db.Db)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to calculate FIFO cost for new product",
		})
	}

	service := model.Service{
		VisitationID:   visitation.ID,
		ProductID:      product.ID,
		SellQuantity:   order.Quantity,
		TotalCost:      totalCost,
		TotalFIFO_Cost: totalFIFO_Cost,
		NetPrice:       totalCost,
		UseTime:        visitation.UseTime,
		Status:         "draft",
	}

	if order.Status != nil {
		service.Status = *order.Status
	}

	if order.DeletedAt != nil {
		service.DeletedAt = gorm.DeletedAt{Time: *order.DeletedAt, Valid: true}
	}

	if err := db.Db.Create(&service).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create service",
		})
	}
	broadcastPOSUpdate(visitation.DivisionID, "order-created")

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Order created successfully",
		"service": service,
	})
}

func legacyCalculateFIFO(productId uint, quantity int, db *gorm.DB) (float64, error) {
	shouldCutStock, err := ShouldManageStock(productId, db)
	if err != nil {
		return 0, err
	}

	// ถ้า category ไม่ได้เปิดใช้ stock → ไม่ต้องตัด stock จริง
	if !shouldCutStock {
		return 0, nil
	}
	var totalFIFO_Cost float64
	remainingQty := quantity

	// Query stock entries ของ product นั้น ๆ เรียงจาก entry ที่เก่าสุด
	var stockEntries []model.StockEntry
	db.Where("product_id = ? AND remaining_qty > 0", productId).Order("entry_date ASC").Find(&stockEntries)

	for _, entry := range stockEntries {
		if remainingQty <= 0 {
			break
		}

		if entry.RemainingQty >= remainingQty {
			// ตัด stock จากล็อตนี้และคำนวณต้นทุน FIFO
			totalFIFO_Cost += float64(remainingQty) * entry.CostPerUnit
			entry.RemainingQty -= remainingQty
			remainingQty = 0
			db.Save(&entry) // อัปเดตจำนวน stock ที่เหลือ
		} else {
			// ตัด stock ทั้งหมดจากล็อตนี้
			totalFIFO_Cost += float64(entry.RemainingQty) * entry.CostPerUnit
			remainingQty -= entry.RemainingQty
			entry.RemainingQty = 0
			db.Save(&entry) // ล็อตนี้หมดแล้ว
		}
	}

	if remainingQty > 0 {
		return 0, fmt.Errorf("not enough stock to cover the order")
	}

	return totalFIFO_Cost, nil
}
func legacyReturnStockFIFO(productId uint, quantity int, db *gorm.DB) (float64, error) {
	shouldCutStock, err := ShouldManageStock(productId, db)
	if err != nil {
		return 0, err
	}

	// ถ้า product นี้ไม่ใช่ stock item ก็ไม่ต้องคืน stock
	if !shouldCutStock {
		return 0, nil
	}
	remainingQty := quantity
	totalReturnedCost := 0.0

	// Query stock entries ของ product นั้น ๆ เรียงจาก entry ที่เก่าสุดไปใหม่สุด (FIFO)
	var stockEntries []model.StockEntry
	db.Where("product_id = ? AND remaining_qty < quantity", productId).Order("entry_date desc").Find(&stockEntries)

	for _, entry := range stockEntries {
		if remainingQty <= 0 {
			break
		}

		// คำนวณจำนวนที่คืนกลับเข้าไปในล็อต
		qtyToReturn := entry.Quantity - entry.RemainingQty

		if qtyToReturn >= remainingQty {
			// คืนสินค้าทั้งหมดในล็อตนี้
			entry.RemainingQty += remainingQty
			totalReturnedCost += float64(remainingQty) * entry.CostPerUnit
			remainingQty = 0
		} else {
			// คืนสินค้าบางส่วนในล็อตนี้
			entry.RemainingQty += qtyToReturn
			totalReturnedCost += float64(qtyToReturn) * entry.CostPerUnit
			remainingQty -= qtyToReturn
		}

		// อัปเดตจำนวน stock ในฐานข้อมูล
		db.Save(&entry)
	}

	if remainingQty > 0 {
		return 0, fmt.Errorf("unable to return all stock")
	}

	return totalReturnedCost, nil
}

type ChangeTableRequest struct {
	NewTableID uint `json:"newTableID"` // ID ของโต๊ะใหม่ที่ส่งมาจาก frontend
}

func ChangeTable(c *fiber.Ctx) error {
	uuid := c.Params("uuid")

	var request ChangeTableRequest
	if err := c.BodyParser(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}
	if request.NewTableID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "New table is required"})
	}

	posMutationMu.Lock()
	defer posMutationMu.Unlock()

	var visitation model.Visitation
	var oldTableID uint
	err := db.Db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("uuid = ? AND is_active = 1 AND (is_paid = 0 OR is_paid = 2)", uuid).
			First(&visitation).Error; err != nil {
			return err
		}
		oldTableID = visitation.TableID

		if oldTableID == request.NewTableID {
			return errors.New("source and destination tables are the same")
		}

		var sourceTable model.SettingTable
		if err := tx.First(&sourceTable, oldTableID).Error; err != nil {
			return errTableNotFound
		}

		var destinationTable model.SettingTable
		if err := tx.First(&destinationTable, request.NewTableID).Error; err != nil {
			return errTableNotFound
		}
		if destinationTable.DivisionID != visitation.DivisionID {
			return errors.New("destination table is not in the same division")
		}
		if math.Abs(sourceTable.Price-destinationTable.Price) > 0.000001 ||
			math.Abs(sourceTable.Price2-destinationTable.Price2) > 0.000001 {
			return errTablePriceMismatch
		}

		var activeCount int64
		if err := tx.Model(&model.Visitation{}).
			Where("table_id = ? AND is_active = 1 AND (is_paid = 0 OR is_paid = 2) AND deleted_at IS NULL", request.NewTableID).
			Count(&activeCount).Error; err != nil {
			return err
		}
		if activeCount > 0 {
			return errTableAlreadyActive
		}

		return tx.Model(&model.Visitation{}).
			Where("id = ? AND table_id = ? AND is_active = 1", visitation.ID, oldTableID).
			Update("table_id", request.NewTableID).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Record not found"})
	}
	if errors.Is(err, errTableNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Destination table not found"})
	}
	if errors.Is(err, errTableAlreadyActive) {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Destination table is already active"})
	}
	if errors.Is(err, errTablePriceMismatch) {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Destination table price does not match source table"})
	}
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	broadcastPOSUpdate(visitation.DivisionID, "table-moved")

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message":   "Table changed successfully",
		"relay_off": publishTableRelayByID(oldTableID, false, "change-table-off"),
		"relay_on":  publishTableRelayByID(request.NewTableID, true, "change-table-on"),
	})
}

func CalculateGameFee(timeInSeconds int64, price float64, price2 float64, isDiscounted bool) float64 {
	ratePerHour := price
	if isDiscounted {
		ratePerHour = price2
	}

	ratePerMinute := ratePerHour / 60
	totalMinutes := int(math.Ceil(float64(timeInSeconds) / 60))

	var fee float64

	if isDiscounted && totalMinutes <= 60 {
		fee = ratePerHour
	} else if totalMinutes <= 30 {
		fee = ratePerMinute * 30
	} else if totalMinutes <= 60 {
		fee = ratePerHour
	} else {
		extraMinutes := totalMinutes - 60
		fee = ratePerHour + float64(extraMinutes)*ratePerMinute
	}

	return math.Ceil(fee) // ปัดขึ้น
}

func legacyShouldManageStock(productID uint, tx *gorm.DB) (bool, error) {
	var result struct {
		IsStock uint8 `gorm:"column:is_stock"`
	}

	err := tx.Table("products p").
		Select("COALESCE(c.is_stock, 0) as is_stock").
		Joins("LEFT JOIN categories c ON c.id = p.category_id").
		Where("p.id = ?", productID).
		Scan(&result).Error

	if err != nil {
		return false, err
	}

	return result.IsStock == 1, nil
}
