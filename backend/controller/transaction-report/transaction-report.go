package transactionreport

import (
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
)

// ใช้ GORM struct
type ProductInfo struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

func ListProducts(c *fiber.Ctx) error {
	fmt.Println("📦 Loading Product List")

	var results []ProductInfo

	query := `
        SELECT 
            p.id,
            p.name
        FROM products p
        LEFT JOIN stock_entries se ON se.product_id = p.id AND se.deleted_at IS NULL
        WHERE p.is_active = 1 AND p.category_id != 3 AND p.id != 1
        GROUP BY p.id, p.name
        ORDER BY p.id ASC
    `

	if err := db.Db.Raw(query).Scan(&results).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"data": results,
	})
}

type ProductTransaction struct {
	ID             uint    `json:"id"`
	Name           string  `json:"name"`
	TotalRemaining float64 `json:"total_remaining"`
	TotalValue     float64 `json:"total_value"`
	ReceivedDate   string  `json:"received_date"` // รูปแบบ YYYY-MM-DD
}
type TransactionSummary struct {
	Date           string  `json:"date"`
	ReceiveQty     float64 `json:"receive_qty"`
	ReceiveCost    float64 `json:"receive_cost"`
	ReceiveValue   float64 `json:"receive_value"`
	SellQty        float64 `json:"sell_qty"`
	SellCost       float64 `json:"sell_cost"`
	SellValue      float64 `json:"sell_value"`
	RemainingQty   float64 `json:"remaining_qty"`   // ✅ ต้องใส่เพิ่ม
	RemainingCost  float64 `json:"remaining_cost"`  // ✅
	RemainingValue float64 `json:"remaining_value"` // ✅
}

func SearchProductTransactions(c *fiber.Ctx) error {
	fmt.Println("📦 Loading Product Transactions")
	fmt.Println("🔍 Query Params:", c.Queries())

	month := c.Query("month")       // เช่น 2025-04
	productID := c.Query("product") // เป็น ID

	if month == "" || productID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "month and product_id are required",
		})
	}

	pid, err := strconv.Atoi(productID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid product id"})
	}

	firstDayOfMonth := month + "-01"

	// ===== 1. ดึงยอดรับสะสม + วันเริ่มมี stock =====
	type FirstStockSummary struct {
		TotalReceiveQty   float64
		TotalReceiveValue float64
		FirstStockDate    string
	}
	var firstSummary FirstStockSummary

	err = db.Db.Raw(`
		SELECT 
			IFNULL(SUM(p.quantity), 0) AS total_receive_qty,
			IFNULL(SUM(p.total_price), 0) AS total_receive_value,
			MIN(pr.received_date) AS first_stock_date
		FROM product_receipt_items p
		LEFT JOIN product_receipts pr ON p.receipt_id = pr.id 
		WHERE 
			substr(p.created_at, 1, 10) < ?
			AND p.is_active = 1
			AND p.deleted_at IS NULL
			AND p.product_id = ?
			AND pr.is_active = 1
			AND pr.deleted_at IS NULL
	`, firstDayOfMonth, pid).Scan(&firstSummary).Error

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	firstStockDate := firstSummary.FirstStockDate
	if firstStockDate == "" {
		firstStockDate = "1970-01-01"
	}
	fmt.Println("🗓️ First Stock Date:", firstStockDate)

	// ===== 2. ดึงยอดขายสะสมตั้งแต่วันเริ่ม stock ถึงก่อนเดือนปัจจุบัน =====
	type SellSummary struct {
		TotalSellQty   float64
		TotalSellValue float64
	}
	var sellSummary SellSummary

	err = db.Db.Raw(`
		SELECT 
			IFNULL(SUM(s.sell_quantity), 0) as total_sell_qty,
			IFNULL(SUM(s.total_fifo_cost), 0) as total_sell_value
		FROM services s
		INNER JOIN visitations v ON v.id = s.visitation_id
		WHERE substr(v.visit_date, 1, 10) >= ?
			AND substr(v.visit_date, 1, 10) < ?
			AND s.deleted_at IS NULL
			AND s.status = 'paid'
			AND v.deleted_at IS NULL
			AND s.product_id = ?
	`, firstStockDate, firstDayOfMonth, pid).Scan(&sellSummary).Error
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	remainQty := firstSummary.TotalReceiveQty - sellSummary.TotalSellQty
	// remainValue := remainQty * firstSummary.TotalReceiveValue / firstSummary.TotalReceiveQty
	remainValue := firstSummary.TotalReceiveValue - sellSummary.TotalSellValue

	// ===== 3. ดึงข้อมูลรับ/ขายของเดือนนี้ =====
	var results []TransactionSummary

	query := `
		SELECT 
			substr(p.created_at, 1, 10) AS date,
			IFNULL(SUM(p.quantity), 0) AS receive_qty,
			IFNULL(ROUND(AVG(p.unit_price), 4), 0) AS receive_cost,
			IFNULL(SUM(p.total_price), 0) AS receive_value,
			0 AS sell_qty,
			0 AS sell_cost,
			0 AS sell_value
		FROM product_receipt_items p
		LEFT JOIN product_receipts pr ON p.receipt_id = pr.id
		WHERE 
			p.is_active = 1
			AND p.deleted_at IS NULL
			AND pr.is_active = 1
			AND pr.deleted_at IS NULL
			AND substr(p.created_at, 1, 7) = ?
			AND p.product_id = ?
		GROUP BY substr(p.created_at, 1, 10)

		UNION ALL

		SELECT 
			substr(v.visit_date, 1, 10) AS date,
			0 AS receive_qty,
			0 AS receive_cost,
			0 AS receive_value,
			IFNULL(SUM(s.sell_quantity), 0) AS sell_qty,
			IFNULL(ROUND(sum(s.total_fifo_cost) / sum(s.sell_quantity), 4), 0) AS sell_cost,
			IFNULL(ROUND(SUM(s.total_fifo_cost), 4), 0) AS sell_value
		FROM services s
		INNER JOIN visitations v ON v.id = s.visitation_id
		WHERE 
			s.deleted_at IS NULL
			AND s.status = 'paid'
			AND v.deleted_at IS NULL
			AND substr(v.visit_date, 1, 7) = ?
			AND s.product_id = ?
		GROUP BY substr(v.visit_date, 1, 10)

		ORDER BY date ASC
	`

	if err := db.Db.Raw(query, month, pid, month, pid).Scan(&results).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// ===== 4. แสดง Console Table =====
	fmt.Println("═════════════════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("📅 วันที่      | รับเข้า   |  หน่วยรับ  |  มูลค่ารับ   | จ่ายออก  |  หน่วยจ่าย  |   มูลค่าจ่าย  | คงเหลือ  | มูลค่า\n")
	fmt.Println("═════════════════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("ยกมา        |    -    |     -    |     -     |    -    |     -     |      -     | %7.0f | %8.4f\n", remainQty, remainValue)

	remainQtyRunning := remainQty
	remainValueRunning := remainValue
	var dailySummary []TransactionSummary

	for _, r := range results {
		remainQtyRunning += r.ReceiveQty - r.SellQty
		remainValueRunning += r.ReceiveValue - r.SellValue

		fmt.Printf("%-11s | %7.0f | %8.4f | %9.4f | %7.0f | %9.4f | %10.4f | %7.0f | %8.4f\n",
			r.Date,
			r.ReceiveQty,
			r.ReceiveCost,
			r.ReceiveValue,
			r.SellQty,
			r.SellCost,
			r.SellValue,
			remainQtyRunning,
			remainValueRunning,
		)

		d := r
		d.RemainingQty = remainQtyRunning
		d.RemainingValue = remainValueRunning

		if remainQtyRunning > 0 {
			d.RemainingCost = remainValueRunning / remainQtyRunning
		} else {
			d.RemainingCost = 0
		}

		dailySummary = append(dailySummary, d)
	}

	fmt.Println("═════════════════════════════════════════════════════════════════════════════════════════════════════════════")

	return c.JSON(fiber.Map{
		"carry_forward_qty":   remainQty,
		"carry_forward_value": remainValue,
		"data":                dailySummary,
	})

}

// func SearchProductTransactions(c *fiber.Ctx) error {
// 	fmt.Println("📦 Loading Product Transactions")
// 	fmt.Println("🔍 Query Params:", c.Queries())

// 	month := c.Query("month")       // เช่น 2025-04
// 	productID := c.Query("product") // เป็น ID

// 	if month == "" || productID == "" {
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
// 			"error": "month and product_id are required",
// 		})
// 	}

// 	pid, err := strconv.Atoi(productID)
// 	if err != nil {
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid product id"})
// 	}

// 	firstDayOfMonth := month + "-01"

// 	// ===== 1. ดึงวันแรกที่มี stock =====
// 	var firstStockDate string
// 	err = db.Db.Raw(`
// 		SELECT MIN(substr(created_at, 1, 10))
// 		FROM product_receipt_items
// 		WHERE is_active = 1 AND deleted_at IS NULL AND product_id = ?
// 	`, pid).Scan(&firstStockDate).Error

// 	if err != nil {
// 		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
// 	}
// 	if firstStockDate == "" {
// 		firstStockDate = "1970-01-01"
// 	}

// 	fmt.Println("🗓️ First Stock Date:", firstStockDate)

// 	// ===== 2. คำนวณยอดยกมา =====
// 	type totalStruct struct {
// 		TotalReceiveQty   float64
// 		TotalReceiveValue float64
// 		TotalSellQty      float64
// 		TotalSellValue    float64
// 	}
// 	var total totalStruct

// 	// ยอดรับก่อนเดือนนี้
// 	err = db.Db.Raw(`
// 		SELECT
// 			IFNULL(SUM(quantity), 0) as total_receive_qty,
// 			IFNULL(SUM(total_price), 0) as total_receive_value
// 		FROM product_receipt_items
// 		WHERE substr(created_at, 1, 10) < ?
// 			AND is_active = 1
// 			AND deleted_at IS NULL
// 			AND product_id = ?
// 	`, firstDayOfMonth, pid).Scan(&total).Error
// 	if err != nil {
// 		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
// 	}

// 	// ยอดขายนับจาก firstStockDate ถึงก่อนเดือนนี้
// 	err = db.Db.Raw(`
// 		SELECT
// 			IFNULL(SUM(s.sell_quantity), 0) as total_sell_qty,
// 			IFNULL(SUM(s.total_cost), 0) as total_sell_value
// 		FROM services s
// 		INNER JOIN visitations v ON v.id = s.visitation_id
// 		WHERE substr(v.visit_date, 1, 10) >= ?
// 			AND substr(v.visit_date, 1, 10) < ?
// 			AND s.deleted_at IS NULL
// 			AND s.status = 'paid'
// 			AND v.deleted_at IS NULL
// 			AND s.product_id = ?
// 	`, firstStockDate, firstDayOfMonth, pid).Scan(&total).Error
// 	if err != nil {
// 		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
// 	}

// 	remainQty := total.TotalReceiveQty - total.TotalSellQty
// 	remainValue := total.TotalReceiveValue - total.TotalSellValue

// 	// ===== 3. ดึงข้อมูลเดือนนี้ =====
// 	var results []TransactionSummary

// 	query := `
// 		SELECT
// 			substr(created_at, 1, 10) AS date,
// 			IFNULL(SUM(quantity), 0) AS receive_qty,
// 			IFNULL(ROUND(AVG(unit_price), 4), 0) AS receive_cost,
// 			IFNULL(SUM(total_price), 0) AS receive_value,
// 			0 AS sell_qty,
// 			0 AS sell_cost,
// 			0 AS sell_value
// 		FROM product_receipt_items
// 		WHERE
// 			is_active = 1
// 			AND deleted_at IS NULL
// 			AND substr(created_at, 1, 7) = ?
// 			AND product_id = ?
// 		GROUP BY substr(created_at, 1, 10)

// 		UNION ALL

// 		SELECT
// 			substr(v.visit_date, 1, 10) AS date,
// 			0 AS receive_qty,
// 			0 AS receive_cost,
// 			0 AS receive_value,
// 			IFNULL(SUM(s.sell_quantity), 0) AS sell_qty,
// 			IFNULL(ROUND(AVG(s.net_price), 4), 0) AS sell_cost,
// 			IFNULL(ROUND(SUM(s.total_cost), 4), 0) AS sell_value
// 		FROM services s
// 		INNER JOIN visitations v ON v.id = s.visitation_id
// 		WHERE
// 			s.deleted_at IS NULL
// 			AND s.status = 'paid'
// 			AND v.deleted_at IS NULL
// 			AND substr(v.visit_date, 1, 7) = ?
// 			AND s.product_id = ?
// 		GROUP BY substr(v.visit_date, 1, 10)

// 		ORDER BY date ASC
// 	`

// 	if err := db.Db.Raw(query, month, pid, month, pid).Scan(&results).Error; err != nil {
// 		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
// 	}

// 	// ===== 4. แสดงผลใน Console =====
// 	fmt.Println("═════════════════════════════════════════════════════════════════════════════════════════════════════════════")
// 	fmt.Printf("📅 วันที่     | รับเข้า   | หน่วยรับ | มูลค่ารับ   | จ่ายออก | หน่วยจ่าย | มูลค่าจ่าย  | คงเหลือ | มูลค่า\n")
// 	fmt.Println("═════════════════════════════════════════════════════════════════════════════════════════════════════════════")
// 	fmt.Printf("ยกมา         |      -    |    -     |     -      |    -    |     -     |     -      | %7.0f | %8.4f\n", remainQty, remainValue)

// 	// สะสมคงเหลือ
// 	remainQtyRunning := remainQty
// 	remainValueRunning := remainValue

// 	var dailySummary []TransactionSummary

// 	for _, r := range results {
// 		remainQtyRunning += r.ReceiveQty - r.SellQty
// 		remainValueRunning += r.ReceiveValue - r.SellValue

// 		fmt.Printf("%-11s | %7.0f | %8.4f | %9.4f | %7.0f | %9.4f | %10.4f | %7.0f | %8.4f\n",
// 			r.Date,
// 			r.ReceiveQty,
// 			r.ReceiveCost,
// 			r.ReceiveValue,
// 			r.SellQty,
// 			r.SellCost,
// 			r.SellValue,
// 			remainQtyRunning,
// 			remainValueRunning,
// 		)

// 		d := r
// 		dailySummary = append(dailySummary, d)
// 	}

// 	fmt.Println("═════════════════════════════════════════════════════════════════════════════════════════════════════════════")

// 	return c.JSON(fiber.Map{
// 		"data": dailySummary,
// 	})
// }

// func SearchProductTransactions(c *fiber.Ctx) error {
// 	fmt.Println("📦 Loading Product Transactions")
// 	fmt.Println("🔍 Query Params:", c.Queries())

// 	month := c.Query("month")       // เช่น 2025-01
// 	productID := c.Query("product") // ควรเป็น id

// 	if month == "" || productID == "" {
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
// 			"error": "month and product_id are required",
// 		})
// 	}

// 	pid, err := strconv.Atoi(productID)
// 	if err != nil {
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid product id"})
// 	}

// 	var results []TransactionSummary

// 	combinedQuery := `
// 		SELECT
// 			substr(created_at, 1, 10) AS date,
// 			IFNULL(SUM(quantity), 0) AS receive_qty,
// 			IFNULL(ROUND(AVG(unit_price), 4), 0) AS receive_cost,
// 			IFNULL(SUM(total_price), 0) AS receive_value,
// 			0 AS sell_qty,
// 			0 AS sell_cost,
// 			0 AS sell_value
// 		FROM product_receipt_items
// 		WHERE
// 			is_active = 1
// 			AND deleted_at IS NULL
// 			AND substr(created_at, 1, 7) = ?
// 			AND product_id = ?
// 		GROUP BY substr(created_at, 1, 10)

// 		UNION ALL

// 		SELECT
// 			substr(v.visit_date, 1, 10) AS date,
// 			0 AS receive_qty,
// 			0 AS receive_cost,
// 			0 AS receive_value,
// 			IFNULL(SUM(s.sell_quantity), 0) AS sell_qty,
// 			IFNULL(ROUND(AVG(s.net_price), 4), 0) AS sell_cost,
// 			IFNULL(ROUND(SUM(s.total_cost), 4), 0) AS sell_value
// 		FROM services s
// 		INNER JOIN visitations v ON v.id = s.visitation_id
// 		WHERE
// 			s.deleted_at IS NULL
// 			AND s.status = 'paid'
// 			AND v.deleted_at IS NULL
// 			AND substr(v.visit_date, 1, 7) = ?
// 			AND s.product_id = ?
// 		GROUP BY substr(v.visit_date, 1, 10)

// 		ORDER BY date ASC
// 	`

// 	if err := db.Db.Raw(combinedQuery, month, pid, month, pid).Scan(&results).Error; err != nil {
// 		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
// 	}

// 	// Header
// 	fmt.Println("-----------------------------------------------------------------------------------------------------------")
// 	fmt.Printf("📅 วันที่     | รับเข้า   | หน่วยรับ | มูลค่ารับ   | จ่ายออก | หน่วยจ่าย | มูลค่าจ่าย  | คงเหลือ | มูลค่า\n")
// 	fmt.Println("-----------------------------------------------------------------------------------------------------------")

// 	// Running totals
// 	var totalReceiveQty, totalReceiveValue float64
// 	var totalSellQty, totalSellValue float64

// 	var remainQty float64
// 	var remainValue float64

// 	for _, r := range results {
// 		remainQty += r.ReceiveQty - r.SellQty
// 		remainValue += r.ReceiveValue - r.SellValue

// 		fmt.Printf("%-11s | %7.0f | %8.4f | %9.4f | %7.0f | %9.4f | %10.4f | %7.0f | %8.4f\n",
// 			r.Date,
// 			r.ReceiveQty,
// 			r.ReceiveCost,
// 			r.ReceiveValue,
// 			r.SellQty,
// 			r.SellCost,
// 			r.SellValue,
// 			remainQty,
// 			remainValue,
// 		)

// 		totalReceiveQty += r.ReceiveQty
// 		totalReceiveValue += r.ReceiveValue
// 		totalSellQty += r.SellQty
// 		totalSellValue += r.SellValue
// 	}
// 	fmt.Println("-----------------------------------------------------------------------------------------------------------")
// 	fmt.Printf("รวมทั้งหมด  | %7.0f |          | %9.4f | %7.0f |          | %10.4f | %7.0f | %8.4f\n",
// 		totalReceiveQty,
// 		totalReceiveValue,
// 		totalSellQty,
// 		totalSellValue,
// 		remainQty,
// 		remainValue,
// 	)
// 	fmt.Println("-----------------------------------------------------------------------------------------------------------")

// 	return c.JSON(fiber.Map{
// 		"data": results,
// 	})
// }

// func SearchProductTransactions(c *fiber.Ctx) error {

// 	fmt.Println("📦 Loading Product Transactions")
// 	fmt.Println("🔍 Query Params:", c.Queries())

// 	month := c.Query("month")       // เช่น 2025-01
// 	productID := c.Query("product") // ชื่อสินค้า (ต้องแปลงไปหา id ก่อน หรือใช้ id แทนจะง่ายกว่า)

// 	if month == "" || productID == "" {
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
// 			"error": "month and product_id are required",
// 		})
// 	}
// 	pid, err := strconv.Atoi(productID)
// 	if err != nil {
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid product id"})
// 	}

// 	// 2. Query รายการรับ
// 	var receiveData []TransactionSummary
// 	receiveQuery := `
// 		SELECT
// 			strftime('%d/%m/%y', created_at) as date,
// 			SUM(quantity) as receive_qty,
// 			AVG(unit_price) as receive_cost,
// 			SUM(total_price) as receive_value,
// 			0 as sell_qty,
// 			0 as sell_cost,
// 			0 as sell_value
// 		FROM product_receipt_items
// 		WHERE is_active = 1
// 		  AND deleted_at IS NULL
// 		  AND strftime('%Y-%m', created_at) = ?
// 		  AND product_id = ?
// 		GROUP BY date
// 		ORDER BY date ASC
// 	`
// 	if err := db.Db.Raw(receiveQuery, month, pid).Scan(&receiveData).Error; err != nil {
// 		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
// 	}
// 	// 3. Query รายการจ่าย
// 	var sellData []TransactionSummary
// 	sellQuery := `
// 	SELECT
// 		substr(v.visit_date, 1, 10) AS date,
// 		0 AS receive_qty,
// 		0 AS receive_cost,
// 		0 AS receive_value,
// 		IFNULL(SUM(s.sell_quantity), 0) AS sell_qty,
// 		IFNULL(ROUND(AVG(s.net_price), 4), 0) AS sell_cost,
// 		IFNULL(ROUND(SUM(s.total_cost), 4), 0) AS sell_value
// 	FROM services s
// 	INNER JOIN visitations v ON v.id = s.visitation_id
// 	WHERE
// 		s.deleted_at IS NULL
// 		AND s.status = 'paid'
// 		AND v.deleted_at IS NULL
// 		AND substr(v.visit_date, 1, 7) = ?
// 		AND s.product_id = ?
// 	GROUP BY substr(v.visit_date, 1, 10)
// 	ORDER BY substr(v.visit_date, 1, 10) ASC
// `

// 	if err := db.Db.Raw(sellQuery, month, productID).Scan(&sellData).Error; err != nil {
// 		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
// 	}

// 	fmt.Println("══════════════════════════════════════════════════")
// 	fmt.Println("🧾 รายการจ่ายออก (Sell Data):")
// 	fmt.Println("══════════════════════════════════════════════════")
// 	for _, s := range sellData {
// 		fmt.Printf("📅 %s | 🛒 %.0f ชิ้น | 💰 %.4f บาท/หน่วย | 💸 %.4f บาท\n",
// 			s.Date, s.SellQty, s.SellCost, s.SellValue)
// 	}
// 	fmt.Println("══════════════════════════════════════════════════")

// 	// 4. รวมข้อมูล ทั้งรับ + จ่าย
// 	combined := make(map[string]*TransactionSummary)

// 	for _, r := range receiveData {
// 		date := r.Date
// 		if _, ok := combined[date]; !ok {
// 			combined[date] = &TransactionSummary{Date: date}
// 		}
// 		combined[date].ReceiveQty += r.ReceiveQty
// 		combined[date].ReceiveCost = r.ReceiveCost
// 		combined[date].ReceiveValue += r.ReceiveValue
// 	}

// 	for _, s := range sellData {
// 		date := s.Date
// 		if _, ok := combined[date]; !ok {
// 			combined[date] = &TransactionSummary{Date: date}
// 		}
// 		combined[date].SellQty += s.SellQty
// 		combined[date].SellCost = s.SellCost
// 		combined[date].SellValue += s.SellValue
// 	}

// 	// 5. แปลงกลับเป็น slice เพื่อ return
// 	var results []TransactionSummary
// 	for _, val := range combined {
// 		results = append(results, *val)
// 	}

// 	return c.JSON(fiber.Map{
// 		"data": results,
// 	})
// }
