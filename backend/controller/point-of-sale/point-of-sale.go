package pointofsale

import (
	"errors" // สำหรับการใช้งาน errors.Is
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
	"golang.org/x/crypto/bcrypt"

	"gorm.io/gorm" // สำหรับการใช้ gorm.ErrRecordNotFound
)

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
		UseTime        time.Time `json:"use_time"`
		PauseTime      time.Time `json:"pause_time"`
		PausedDuration int64     `json:"paused_duration"`
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
            v.paused_duration,
			v.pause_time,
            v.uuid,
			v.id as visitation_id
        FROM setting_tables st
        LEFT JOIN visitations v ON st.id = v.table_id AND v.is_active = 1 AND (v.is_paid = 0 OR v.is_paid = 2) AND v.deleted_at IS NULL
		 WHERE st.division_id = ?
    
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
		// สร้าง Visitation ใหม่
		visitation := model.Visitation{
			TableID:    json.TableID,
			DivisionID: 1,
			VisitDate:  time.Now().Truncate(24 * time.Hour),
			StartTime:  time.Now(),
			UseTime:    time.Now(),
			PauseTime:  time.Now(),
			TotalCost:  0,
			NetPrice:   0,
			IsPaid:     0,
			IsVisit:    0,
			IsActive:   1,
		}

		result := db.Db.Create(&visitation)
		if result.Error != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
		}

		// ค้นหา Product ที่เป็นค่าโต๊ะสนุ๊ก
		var product model.Product
		if err := db.Db.Where("is_snooker_time = ?", true).First(&product).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to find snooker product"})
		}

		// ค้นหาราคาจาก SettingTable
		var settingTable model.SettingTable
		if err := db.Db.Where("id = ?", json.TableID).First(&settingTable).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to find setting table"})
		}

		// สร้าง Service ใหม่สำหรับค่าโต๊ะสนุ๊ก
		service := model.Service{
			VisitationID:   visitation.ID,
			ProductID:      product.ID,
			SellQuantity:   1, // ค่าเริ่มต้นคือ 1 ชั่วโมง อาจเปลี่ยนแปลงได้เมื่อมีการคำนวณเวลาจริง
			TotalFIFO_Cost: settingTable.Price,
			TotalCost:      settingTable.Price, // คำนวณราคาจากราคาปกติ
			NetPrice:       settingTable.Price, // ยังไม่มีการหักส่วนลด
			UseTime:        visitation.UseTime, // เก็บ UseTime จาก Visitation
			Status:         "draft",
		}

		if err := db.Db.Create(&service).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create service"})
		}

		var table []struct {
			model.SettingTable
			Status         string    `json:"status"`
			StartTime      time.Time `json:"start_time"`
			UseTime        time.Time `json:"use_time"`
			PauseTime      time.Time `json:"pause_time"`
			PausedDuration int64     `json:"paused_duration"`
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
        v.use_time,
        v.paused_duration,
        v.pause_time,
        v.uuid,
        v.id as visitation_id
    FROM setting_tables st
    LEFT JOIN visitations v ON st.id = v.table_id AND v.is_active = 1 AND (v.is_paid = 0 OR v.is_paid = 2) AND v.deleted_at IS NULL
     WHERE v.uuid = ?

`, visitation.Uuid).Scan(&table)

		if result2.Error != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result2.Error.Error()})
		}

		return c.JSON(fiber.Map{
			"message":    "Data processed successfully",
			"uuid":       visitation.Uuid,
			"code":       visitation.Code,
			"start_time": visitation.StartTime,
			"table":      table,
		})
	} else if json.Status == "close" {
		// ปิดโต๊ะที่มีอยู่โดยตั้งค่า is_active = 0
		result := db.Db.Model(&model.Visitation{}).
			Where("table_id = ? AND is_active = 1", json.TableID).
			Update("is_active", 0)

		if result.Error != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
		}

		return c.JSON(fiber.Map{
			"message": "Table closed successfully",
		})
	}

	return c.JSON(fiber.Map{"message": "Invalid status"})
}

type UpdateUseTimeBody struct {
	UUID string `json:"uuid"`
}

func UpdateUseTime(c *fiber.Ctx) error {
	var json UpdateUseTimeBody

	if err := c.BodyParser(&json); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	var visitation model.Visitation
	if err := db.Db.Where("uuid = ?", json.UUID).First(&visitation).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Record not found"})
	}

	visitation.UseTime = visitation.UseTime.Add(time.Minute)
	if err := db.Db.Save(&visitation).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "UseTime updated successfully"})
}

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

func PaymentStore(c *fiber.Ctx) error {
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

	// ส่ง response กลับไปยัง frontend
	return c.JSON(fiber.Map{
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
func UpdatePausedDurationTime(c *fiber.Ctx) error {
	fmt.Println(string(c.Body())) // Log body ที่ส่งเข้ามาเพื่อ debug
	var request struct {
		UUID           string `json:"uuid"`
		PausedDuration int64  `json:"pausedDuration"`
		PauseTime      string `json:"pauseTime"`
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

	if request.PausedDuration == 0 {
		// User resumed the game
		// Convert visitation.PausedDuration to time.Duration
		pausedDuration := time.Duration(visitation.PausedDuration) * time.Second

		// Calculate the actual duration since start_time
		duration := time.Since(visitation.StartTime)
		actualDuration := duration - pausedDuration

		// Update PauseTime to reflect the actual pause time in the "2000-01-01 00:00:00" format
		visitation.PauseTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC).Add(actualDuration)
		visitation.PausedDuration = 0
	} else {
		// User paused the game
		visitation.PausedDuration = request.PausedDuration

		// Reset PauseTime to "2000-01-01 00:00:00"
		visitation.PauseTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	if err := db.Db.Save(&visitation).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update visitation",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Paused duration updated successfully",
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
	println("Hashed Password from DB:", user.Password)
	println("Password from Request:", request.Password)
	fmt.Printf("User: %+v\n", user)

	// ตรวจสอบรหัสผ่านด้วย bcrypt
	if !CheckPasswordHash(request.Password, user.Password) {
		println("Password mismatch")
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
	result := db.Db.Model(&model.Visitation{}).
		Where("table_id = ? AND is_active = 1", request.TableID).
		Update("is_active", 0)

	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	}

	return c.JSON(fiber.Map{
		"message": "Table closed successfully",
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
	println("Hashed Password from DB:", user.Password)
	println("Password from Request:", request.Password)
	fmt.Printf("User: %+v\n", user)

	// ตรวจสอบรหัสผ่านด้วย bcrypt
	if !CheckPasswordHash(request.Password, user.Password) {
		println("Password mismatch")
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

	// ถ้ารหัสผ่านถูกต้อง ปิดโต๊ะที่มีอยู่โดยตั้งค่า is_active = 0
	result := db.Db.Model(&model.Visitation{}).
		Where("table_id = ? AND is_active = 1", request.TableID).
		Update("is_active", 0)

	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	}

	return c.JSON(fiber.Map{
		"message": "Table closed successfully",
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
func OrderStore(c *fiber.Ctx) error {
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

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Order created successfully",
		"service": service,
	})
}

func CalculateFIFO(productId uint, quantity int, db *gorm.DB) (float64, error) {
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
func ReturnStockFIFO(productId uint, quantity int, db *gorm.DB) (float64, error) {
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
	// ดึง UUID จาก path parameter
	uuid := c.Params("uuid")

	// ตรวจสอบและดึงข้อมูล visitation จาก UUID
	var visitation model.Visitation
	if err := db.Db.Where("uuid = ?", uuid).First(&visitation).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Record not found"})
	}

	// อ่านค่า ID โต๊ะใหม่จาก body
	var request ChangeTableRequest
	if err := c.BodyParser(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// อัปเดต table_id เป็น ID โต๊ะใหม่
	visitation.TableID = request.NewTableID
	if err := db.Db.Save(&visitation).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update record"})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Table changed successfully"})
}
