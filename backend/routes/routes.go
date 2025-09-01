package routes

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	// "github.com/go-redis/redis"
	// "github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/rainza999/fiber-test/controller/auth"
	"github.com/rainza999/fiber-test/controller/division"

	helper "github.com/rainza999/fiber-test/controller/helper"
	pointofsale "github.com/rainza999/fiber-test/controller/point-of-sale"
	"github.com/rainza999/fiber-test/controller/role"
	settingpointofsale "github.com/rainza999/fiber-test/controller/setting-point-of-sale"
	settingsystem "github.com/rainza999/fiber-test/controller/setting-system"
	settingtable "github.com/rainza999/fiber-test/controller/setting-table"
	"github.com/rainza999/fiber-test/controller/user"

	receiptreport "github.com/rainza999/fiber-test/controller/receipt-report"
	saleofreport "github.com/rainza999/fiber-test/controller/sale-of-report"
	supplier "github.com/rainza999/fiber-test/controller/supplier"
	transactionreport "github.com/rainza999/fiber-test/controller/transaction-report"

	license "github.com/rainza999/fiber-test/controller/license"

	receipt "github.com/rainza999/fiber-test/controller/receipt"

	"github.com/rainza999/fiber-test/controller/category"
	"github.com/rainza999/fiber-test/controller/dashboard"
	"github.com/rainza999/fiber-test/controller/product"

	middleware "github.com/rainza999/fiber-test/controller/middleware"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
)

// func getMenuFromRole(c *fiber.Ctx) error {
// 	tokenFromCookie := c.Cookies("jwt")

// 	token, err := jwt.Parse(tokenFromCookie, func(token *jwt.Token) (interface{}, error) {
// 		return []byte("your-secret-key"), nil // นี่คือ secret key ที่ใช้ในการเข้ารหัส token
// 	})

// 	if err != nil {
// 		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized: Failed to parse token")
// 	}

// 	if token.Valid {
// 		// Access token claims
// 		claims, ok := token.Claims.(jwt.MapClaims)
// 		if !ok {
// 			return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized: Invalid token claims")
// 		}

// 		// ดึง role_id จาก token claims
// 		if roleID, ok := claims["role_id"].(float64); ok {
// 			globalCtx := context.Background()

// 			// สร้าง Redis client
// 			client := redis.NewClient(&redis.Options{
// 				Addr:     "redis:6379",
// 				Password: "", // ใส่ password ถ้ามี
// 				DB:       0,
// 			})

// 			// เรียกฟังก์ชัน GetMenuCache จาก helper.go เพื่อตรวจสอบว่า cache มีอยู่หรือไม่
// 			cachedData, err := helper.GetMenuCache(globalCtx, client, int(roleID))
// 			if err != nil {
// 				return c.Status(fiber.StatusInternalServerError).SendString("Error fetching cache")
// 			}

// 			if cachedData == nil {
// 				// ถ้าไม่เจอ cache ดึงข้อมูลจากฐานข้อมูลแล้วสร้าง cache ใหม่
// 				if err := helper.SetCacheMenu(globalCtx, client, int(roleID)); err != nil {
// 					return c.Status(fiber.StatusInternalServerError).SendString("Error setting cache")
// 				}
// 				// หลังจากสร้าง cache ใหม่ ดึง cache ข้อมูลกลับมาอีกครั้ง
// 				cachedData, err = helper.GetMenuCache(globalCtx, client, int(roleID))
// 				if err != nil || cachedData == nil {
// 					return c.Status(fiber.StatusInternalServerError).SendString("Error fetching cache after setting it")
// 				}
// 			}

// 			// ส่ง cache กลับไปยัง client
// 			return c.Status(200).JSON(fiber.Map{
// 				"status": "ok",
// 				"menus":  string(cachedData),
// 			})

// 		} else {
// 			log.Println("Invalid role ID type in claims")
// 		}
// 	}

// 	// กรณีไม่ตรงค่าที่คาดหวัง
// 	return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized: Invalid token")
// }

// Middleware สำหรับตรวจสอบคุกกี้ "jwt"
func verifyJWT(c *fiber.Ctx) error {
	fmt.Println("verifyJWT")

	// ✅ อ่านจาก Authorization Header แทน cookie
	authHeader := c.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"authenticated": false,
			"message":       "Unauthorized: Missing token header",
		})
	}
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

	// ✅ แกะ token
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		return []byte("my-secret-key"), nil // ใช้ secret ที่ตรงกับตอน login
	})

	if err != nil || !token.Valid {
		log.Printf("Invalid token. Reason: %v\nToken: %v\n", err, token)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"authenticated": false,
			"message":       fmt.Sprintf("Unauthorized: Invalid token. Reason: %v", err),
		})
	}

	// if err != nil || !token.Valid {
	// 	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
	// 		"authenticated": false,
	// 		"message":       "Unauthorized: Invalid token",
	// 	})
	// }

	// ✅ แยก claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"authenticated": false,
			"message":       "Unauthorized: Invalid claims",
		})
	}
	// ดึง user_id และ role_id จาก claims
	userID, _ := claims["user_id"].(float64)

	// ดึงข้อมูลของ User จากฐานข้อมูล โดยใช้ userID ที่ได้จาก claims
	var user model.User
	if err := db.Db.Preload("Employee").Preload("Role").First(&user, int(userID)).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"authenticated": false,
			"message":       "User not found",
		})
	}

	// ดึงข้อมูล permission จาก role_has_permissions และ permissions โดยใช้ LEFT JOIN
	var permissions []string
	if err := db.Db.Table("role_has_permissions").
		Select("permissions.name").
		Joins("LEFT JOIN permissions ON role_has_permissions.permission_id = permissions.id").
		Where("role_has_permissions.role_id = ?", user.RoleID).
		Pluck("permissions.name", &permissions).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"authenticated": false,
			"message":       "Error fetching permissions",
		})
	}

	// สร้าง context และ redis client
	// globalCtx := context.Background()
	// client := redis.NewClient(&redis.Options{
	// 	Addr:     "redis:6379",
	// 	Password: "", // ใส่ password ถ้ามี
	// 	DB:       0,
	// })

	// สร้างคีย์จาก user.RoleID
	// redisKey := fmt.Sprintf("menus_%d", user.RoleID)
	// ใช้คำสั่ง GET เพื่อดึงข้อมูลจาก Redis

	// ใช้คำสั่ง GET เพื่อดึงข้อมูลจาก Redis
	fmt.Println("Attempting to retrieve cached data...")
	// cachedData, err := helper.GetMenuData(globalCtx, int(user.RoleID))

	// menuByRole := helper.GetMenuByRole(int(user.RoleID))
	// if menuByRole == nil {
	// 	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
	// 		"authenticated": false,
	// 		"message":       "Error fetching menu data",
	// 	})
	// }
	menuData, err := helper.GetMenuData(int(user.RoleID))
	if err != nil {
		// จัดการข้อผิดพลาด เช่น log หรือส่ง response
		log.Println("Error fetching menu data:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"authenticated": false,
			"message":       "Error fetching menu data",
		})
	}
	menuDataJSON, err := json.Marshal(menuData)
	if err != nil {
		log.Println("Error marshalling menu data:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"authenticated": false,
			"message":       "Error marshalling menu data",
		})
	}

	// Debugging output
	fmt.Println("verifyJWT")
	fmt.Println("Final cachedData:", string(menuDataJSON))
	fmt.Println("end verifyJWT")

	// คืนค่า JSON พร้อมข้อมูลจาก Redis และข้อมูลผู้ใช้
	return c.JSON(fiber.Map{
		"authenticated": true,
		"currentUser": fiber.Map{
			"first_name":  user.Employee.FirstName, // แสดง first name ของ employee
			"last_name":   user.Employee.LastName,  // แสดง last name ของ employee
			"uuid":        user.Employee.Uuid,
			"role":        user.Role.Name,       // แสดงชื่อ role ของผู้ใช้
			"menus":       string(menuDataJSON), // ข้อมูลเมนูที่ดึงมาจาก Redis
			"permissions": permissions,          // ข้อมูลสิทธิ์ที่ดึงมาจากฐานข้อมูล
		},
	})
}

// func verifyJWT(c *fiber.Ctx) error {
// 	// แสดงข้อความเพื่อ debug
// 	fmt.Println("verifyJWT")

// 	// ดึง JWT จาก cookie
// 	tokenFromCookie := c.Cookies("jwt")
// 	fmt.Println(tokenFromCookie)
// 	fmt.Println("end verifyJWT")

// 	// ถ้าไม่มี JWT ในคุกกี้ ให้ส่ง Unauthorized กลับ
// 	if tokenFromCookie == "" {
// 		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
// 			"authenticated": false,
// 			"message":       "Unauthorized: Cookie not found rainrainrain",
// 		})
// 	}

// 	// ตรวจสอบ JWT token
// 	token, err := jwt.Parse(tokenFromCookie, func(token *jwt.Token) (interface{}, error) {
// 		return []byte("your-secret-key"), nil
// 	})

// 	// ถ้า JWT ไม่ถูกต้องหรือเกิด error ให้ส่ง Unauthorized กลับ
// 	if err != nil || !token.Valid {
// 		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
// 			"authenticated": false,
// 			"message":       "Unauthorized: Invalid token",
// 		})
// 	}

// 	// ดึง claims จาก token
// 	claims, ok := token.Claims.(jwt.MapClaims)
// 	if !ok {
// 		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
// 			"authenticated": false,
// 			"message":       "Unauthorized: Invalid claims",
// 		})
// 	}
// 	// ดึง user_id และ role_id จาก claims
// 	userID, _ := claims["user_id"].(float64)

// 	// ดึงข้อมูลของ User จากฐานข้อมูล โดยใช้ userID ที่ได้จาก claims
// 	var user model.User
// 	if err := db.Db.Preload("Employee").Preload("Role").First(&user, int(userID)).Error; err != nil {
// 		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
// 			"authenticated": false,
// 			"message":       "User not found",
// 		})
// 	}

// 	// ดึงข้อมูล permission จาก role_has_permissions และ permissions โดยใช้ LEFT JOIN
// 	var permissions []string
// 	if err := db.Db.Table("role_has_permissions").
// 		Select("permissions.name").
// 		Joins("LEFT JOIN permissions ON role_has_permissions.permission_id = permissions.id").
// 		Where("role_has_permissions.role_id = ?", user.RoleID).
// 		Pluck("permissions.name", &permissions).Error; err != nil {
// 		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
// 			"authenticated": false,
// 			"message":       "Error fetching permissions",
// 		})
// 	}

// 	// สร้าง context และ redis client
// 	// globalCtx := context.Background()
// 	// client := redis.NewClient(&redis.Options{
// 	// 	Addr:     "redis:6379",
// 	// 	Password: "", // ใส่ password ถ้ามี
// 	// 	DB:       0,
// 	// })

// 	// สร้างคีย์จาก user.RoleID
// 	// redisKey := fmt.Sprintf("menus_%d", user.RoleID)
// 	// ใช้คำสั่ง GET เพื่อดึงข้อมูลจาก Redis

// 	// ใช้คำสั่ง GET เพื่อดึงข้อมูลจาก Redis
// 	fmt.Println("Attempting to retrieve cached data...")
// 	// cachedData, err := helper.GetMenuData(globalCtx, int(user.RoleID))

// 	// menuByRole := helper.GetMenuByRole(int(user.RoleID))
// 	// if menuByRole == nil {
// 	// 	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
// 	// 		"authenticated": false,
// 	// 		"message":       "Error fetching menu data",
// 	// 	})
// 	// }
// 	menuData, err := helper.GetMenuData(int(user.RoleID))
// 	if err != nil {
// 		// จัดการข้อผิดพลาด เช่น log หรือส่ง response
// 		log.Println("Error fetching menu data:", err)
// 		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
// 			"authenticated": false,
// 			"message":       "Error fetching menu data",
// 		})
// 	}
// 	menuDataJSON, err := json.Marshal(menuData)
// 	if err != nil {
// 		log.Println("Error marshalling menu data:", err)
// 		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
// 			"authenticated": false,
// 			"message":       "Error marshalling menu data",
// 		})
// 	}

// 	// cachedData, err := helper.GetMenuData(int(user.RoleID))

// 	// if err != nil {
// 	// 	fmt.Println("Error fetching cache:", err)
// 	// 	return c.Status(fiber.StatusInternalServerError).SendString("Error fetching cache")
// 	// }

// 	// if cachedData == nil {
// 	// 	fmt.Println("No cache found, attempting to set cache from database...")
// 	// 	// ถ้าไม่เจอ cache ดึงข้อมูลจากฐานข้อมูลแล้วสร้าง cache ใหม่
// 	// 	if err := helper.SetCacheMenu(globalCtx, client, int(user.RoleID)); err != nil {
// 	// 		fmt.Println("Error setting cache:", err)
// 	// 		return c.Status(fiber.StatusInternalServerError).SendString("Error setting cache")
// 	// 	}

// 	// 	// หลังจากสร้าง cache ใหม่ ดึง cache ข้อมูลกลับมาอีกครั้ง
// 	// 	cachedData, err = helper.GetMenuCache(globalCtx, client, int(user.RoleID))
// 	// 	if err != nil || cachedData == nil {
// 	// 		fmt.Println("Failed to retrieve cache after setting it, fetching directly from database...")

// 	// 		// ถ้ายังไม่สามารถดึงข้อมูลจาก Redis ได้ ให้ดึงข้อมูลจากฐานข้อมูลโดยตรง
// 	// 		menuCacheData, err := helper.SetNotCacheMenu(globalCtx, int(user.RoleID))
// 	// 		if err != nil {
// 	// 			fmt.Println("Error fetching data directly from database:", err)
// 	// 			return c.Status(fiber.StatusInternalServerError).SendString("Error fetching data directly from database")
// 	// 		}

// 	// 		// แปลง `menuCacheData` เป็น JSON []byte
// 	// 		cachedData, err = json.Marshal(menuCacheData)
// 	// 		if err != nil {
// 	// 			fmt.Println("Error marshalling data to JSON:", err)
// 	// 			return c.Status(fiber.StatusInternalServerError).SendString("Error marshalling data to JSON")
// 	// 		}
// 	// 		fmt.Println("Data retrieved and marshalled from database:", string(cachedData))
// 	// 	} else {
// 	// 		fmt.Println("Cache retrieved after setting:", string(cachedData))
// 	// 	}
// 	// } else {
// 	// 	fmt.Println("Cache successfully retrieved:", string(cachedData))
// 	// }

// 	// Debugging output
// 	fmt.Println("verifyJWT")
// 	fmt.Println("Final cachedData:", string(menuDataJSON))
// 	fmt.Println("end verifyJWT")

// 	// คืนค่า JSON พร้อมข้อมูลจาก Redis และข้อมูลผู้ใช้
// 	return c.JSON(fiber.Map{
// 		"authenticated": true,
// 		"currentUser": fiber.Map{
// 			"first_name":  user.Employee.FirstName, // แสดง first name ของ employee
// 			"last_name":   user.Employee.LastName,  // แสดง last name ของ employee
// 			"uuid":        user.Employee.Uuid,
// 			"role":        user.Role.Name,       // แสดงชื่อ role ของผู้ใช้
// 			"menus":       string(menuDataJSON), // ข้อมูลเมนูที่ดึงมาจาก Redis
// 			"permissions": permissions,          // ข้อมูลสิทธิ์ที่ดึงมาจากฐานข้อมูล
// 		},
// 	})
// }

// func checkJWTCookie(c *fiber.Ctx) error {
// 	// ดึงค่าคุกกี้ชื่อ "jwt" จาก request
// 	tokenFromCookie := c.Cookies("jwt")

// 	log.Println("token start")
// 	log.Println(tokenFromCookie)
// 	log.Println("token end")
// 	// ตรวจสอบว่าค่าคุกกี้ถูกส่งมาหรือไม่
// 	if tokenFromCookie == "" {
// 		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized: Cookie not found")
// 	}

// 	token, err := jwt.Parse(tokenFromCookie, func(token *jwt.Token) (interface{}, error) {
// 		return []byte("your-secret-key"), nil // นี่คือ secret key ที่ใช้ในการเข้ารหัส token
// 	})

// 	// // ตรวจสอบว่าไม่มี error และ token ถูกต้อง
// 	// if err == nil && token.Valid {
// 	// 	return c.Next()
// 	// }
// 	if err != nil {
// 		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized: Failed to parse token")
// 	}

// 	// Check if the token is valid
// 	if token.Valid {
// 		// Access token claims
// 		claims, ok := token.Claims.(jwt.MapClaims)
// 		if !ok {
// 			return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized: Invalid token claims")
// 		}

// 		log.Println(claims)
// 		// Access the user ID from claims
// 		if userID, ok := claims["user_id"].(float64); ok {
// 			// ทำอะไรก็ตามที่คุณต้องการกับ userID
// 			log.Println("User ID:", int(userID))

// 			// if err := Helper.getMenuCache; err != nil {
// 			// 	log.Fatal(err)
// 			// }
// 		} else {
// 			log.Println("Invalid user ID type in claims")
// 		}

// 		if roleID, ok := claims["role_id"].(float64); ok {

// 			// Set ค่า role_id ลงใน Locals ของ Fiber context
// 			log.Println(roleID)
// 			log.Println("checkJWTCookie")
// 			c.Locals("roleID", int(roleID))

// 			// เรียกใช้ getMenuCache และเก็บค่าที่ได้
// 			// globalCtx := context.Background()
// 			// client := redis.NewClient(&redis.Options{
// 			// 	Addr:     "redis:6379",
// 			// 	Password: "", // ใส่ password ถ้ามี
// 			// 	DB:       0,
// 			// })
// 			// cachedData, err := helper.GetMenuCache(globalCtx, client, int(roleID))
// 			// if err != nil {
// 			// 	log.Println("if on checkJWTCookie")
// 			// 	log.Fatal(err)
// 			// }

// 			// log.Println("Cached Data:", string(cachedData))

// 		} else {
// 			log.Println("Invalid role ID type in claims")
// 		}

// 		return c.Next()
// 	}

//		// กรณีไม่ตรงค่าที่คาดหวัง
//		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized: Invalid token")
//	}
func checkJWTHeader(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")
	log.Println("Authorization XXX Header:", authHeader)

	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"authenticated": false,
			"message":       "Unauthorized: Missing token header",
		})
	}

	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	log.Println("Parsed Token String:", tokenStr)

	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		// ✅ ป้องกัน alg spoofing
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte("my-secret-key"), nil
	})

	if err != nil || !token.Valid {
		log.Printf("❌ Invalid token. Error: %v, Token valid: %v\n", err, token.Valid)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"authenticated": false,
			"message":       fmt.Sprintf("Unauthorized: Invalid token. Error: %v", err),
		})
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		log.Println("❌ Invalid claims")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"authenticated": false,
			"message":       "Unauthorized: Invalid claims",
		})
	}

	// ✅ Debug claims
	log.Printf("✅ Token Claims: %+v\n", claims)

	// ✅ เก็บลง context
	if roleID, ok := claims["role_id"].(float64); ok {
		c.Locals("roleID", int(roleID))
	}
	if userID, ok := claims["user_id"].(float64); ok {
		c.Locals("userID", int(userID))
	}

	return c.Next()
}

// func Setup(app *fiber.App) {*fiber.Ctx
func Setup(app *fiber.App) {

	// app.Get("/test-cookie", func(c *fiber.Ctx) error {
	// 	// ดึงค่าคุกกี้ชื่อ "jwt" จาก request
	// 	tokenFromCookie := c.Cookies("jwt")

	// 	log.Println("token start111")
	// 	log.Println(tokenFromCookie)
	// 	log.Println("token end111")
	// 	// ตรวจสอบว่าค่าคุกกี้ถูกส่งมาหรือไม่
	// 	if tokenFromCookie == "" {
	// 		return c.Status(fiber.StatusNotFound).SendString("Cookie not found")
	// 	}

	// 	// ส่งค่าคุกกี้กลับไปใน response
	// 	return c.SendString("Cookie value: " + tokenFromCookie)
	// })
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.SendStatus(200)
	})
	app.Post("/login", func(c *fiber.Ctx) error {
		return auth.Login(c)
	})

	app.Post("/refresh-token", func(c *fiber.Ctx) error { return auth.RefreshToken(c) })

	app.Post("/logout", func(c *fiber.Ctx) error {
		// ลบ cookie หรือ session ที่เกี่ยวข้อง
		c.ClearCookie() // ลบ cookies ทั้งหมด

		// Redirect ไปที่หน้า login หรือส่ง response กลับไปที่ฝั่ง client ว่าทำการ logout สำเร็จแล้ว
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"message": "Logout successful",
		})
	})

	// app.Post("/login", auth.Login)

	app.Get("/api/user", auth.Users)

	/*สำหรับ GET VIEW PAGE*/
	// app.Get("")
	app.Get("", func(c *fiber.Ctx) error {
		log.Println("hello")
		// return auth.Login(c, client, ctx)
		return c.JSON(fiber.Map{
			"status": "ok",
		})
	})

	/*สำหรับ GET VIEW PAGE*/
	// // API สำหรับเช็คสิทธิ์การใช้งาน
	// app.Get("/api/check-permission/:permission", func(c *fiber.Ctx) error {
	// 	permission := c.Params("permission")
	// 	userPermissions := getUserPermissions(c)

	// 	if !hasPermission(userPermissions, permission) {
	// 		return c.Status(403).JSON(fiber.Map{"error": "Invalid openingDate format"})
	// 	}

	// 	return c.JSON(fiber.Map{
	// 		"status": "ok",
	// 	})
	// })
	app.Get("/api/verify-jwt", verifyJWT)
	// menu := app.Group("/menu")
	// menu.Get("/data", getMenuFromRole)

	// dashboardsGroup := app.Group("/dashboard", checkJWTCookie)
	dashboardsGroup := app.Group("/dashboard", checkJWTHeader)

	dashboardsGroup.Get("", dashboard.GetView)

	// // Group routes under /users
	// app.Use("/users/*", checkJWTCookie)
	// usersGroup := app.Group("/users")
	// usersGroup.Get("", func(c *fiber.Ctx) error {
	// 	// return middleware.PermissionMiddleware("users-access")(c)

	// 	log.Println("Hello Users")
	// 	return c.JSON(fiber.Map{
	// 		"status": "ok",
	// 	})
	// })

	// ตรวจสอบ JWT Cookie และ PermissionMiddleware ในกลุ่ม /users
	// usersGroup := app.Group("/users", checkJWTCookie, middleware.PermissionMiddleware("users-access"))
	usersGroup := app.Group("/users", checkJWTHeader, middleware.PermissionMiddleware("users-access"))

	usersGroup.Get("", func(c *fiber.Ctx) error {
		log.Println("Hello Users")
		return c.JSON(fiber.Map{
			"status": "ok",
		})
	})
	usersGroup.Get("/create", func(c *fiber.Ctx) error {
		log.Println("Hello Users Create")
		return c.JSON(fiber.Map{
			"status": "ok",
		})
	})
	usersGroup.Get("/:id/edit", func(c *fiber.Ctx) error {
		log.Println("Hello Users Edit")
		return c.JSON(fiber.Map{
			"status": "ok",
		})
	})
	usersGroup.Post("/store", user.Store)
	usersGroup.Get("/:id/edits", user.Edit)
	usersGroup.Put("/:uuid/update", user.Update)
	usersGroup.Get("/anyData", user.AnyData)
	usersGroup.Delete("/:uuid/delete", user.Delete)
	// settingTableGroup := app.Group("/setting-tables", checkJWTCookie, middleware.PermissionMiddleware("setting-table-access"))
	settingTableGroup := app.Group("/setting-tables", checkJWTHeader, middleware.PermissionMiddleware("setting-table-access"))

	settingTableGroup.Get("", func(c *fiber.Ctx) error {
		log.Println("Hello setting table group")
		return c.JSON(fiber.Map{
			"status": "ok",
		})
	})
	settingTableGroup.Get("/anyData", settingtable.AnyData)
	settingTableGroup.Post("/store", settingtable.Store)
	settingTableGroup.Get("/:id/edit", settingtable.Edit)
	settingTableGroup.Put("/:id/update", settingtable.Update)
	//Group routes /roles
	// app.Use("/roles/*", checkJWTCookie)
	app.Use("/roles/*", checkJWTHeader)

	rolesGroup := app.Group("/roles")
	rolesGroup.Get("", func(c *fiber.Ctx) error {
		log.Println("Hello Roles")
		return c.JSON(fiber.Map{
			"status": "ok",
		})
	})
	rolesGroup.Get("/:uuid/edit", func(c *fiber.Ctx) error {
		log.Println("Hello Role edit")
		return c.JSON(fiber.Map{
			"status": "ok",
		})
	})
	rolesGroup.Get("/anyData", role.AnyData)
	rolesGroup.Get("/create/anyData", role.CreateAnyData)
	rolesGroup.Post("/store", role.Store)
	rolesGroup.Get("/:uuid/edits", role.Edit)
	rolesGroup.Put("/update", role.Update)
	rolesGroup.Delete("/:uuid/delete", role.Delete)

	//Group routes /divisions
	// app.Use("/divisions/*", checkJWTCookie)
	app.Use("/divisions/*", checkJWTHeader)

	divisionsGroup := app.Group("/divisions")
	divisionsGroup.Get("", division.GetView)
	divisionsGroup.Get("/anyData", division.AnyData)
	divisionsGroup.Post("/store", division.Store)
	divisionsGroup.Get("/:id/edit", division.Edit)
	divisionsGroup.Put("/:id/update", division.Update)
	divisionsGroup.Delete("/:id/delete", division.Delete)

	// app.Use("/point-of-sales/*", checkJWTCookie)
	app.Use("/point-of-sales/*", checkJWTHeader)

	pointofsalesGroup := app.Group("/point-of-sales")
	pointofsalesGroup.Get("", func(c *fiber.Ctx) error {
		log.Println("Hello Point Of Sales")
		return c.JSON(fiber.Map{
			"status": "ok",
		})
	})
	pointofsalesGroup.Get("/anyData", pointofsale.AnyData)
	pointofsalesGroup.Post("/store/visitation", pointofsale.Store)
	pointofsalesGroup.Post("/api/updateUseTime", pointofsale.UpdateUseTime)
	pointofsalesGroup.Post("/api/verify-password", pointofsale.VerifyPassword)
	pointofsalesGroup.Post("/api/verify-password-and-close-table", pointofsale.VerifyPasswordAndCloseTable)
	pointofsalesGroup.Post("/api/updatePausedDurationTime", pointofsale.UpdatePausedDurationTime)
	pointofsalesGroup.Get("/:uuid/visitation", pointofsale.GetVisitationByUUID)
	pointofsalesGroup.Put("/:uuid/visitation/payment", pointofsale.PaymentStore)
	pointofsalesGroup.Get("/:uuid/visitation/payment-pending", pointofsale.PaymentPending)
	pointofsalesGroup.Post("/:uuid/visitation/order/store", pointofsale.OrderStore)
	pointofsalesGroup.Put("/:uuid/visitation/changeTable", pointofsale.ChangeTable)
	// pointofsalesGroup.Get("/:uuid/live", pointofsale.Live)
	pointofsalesGroup.Get("/live", pointofsale.Live)

	categoriesGroup := app.Group("/categories")
	categoriesGroup.Get("/anyData", category.AnyData)
	categoriesGroup.Post("/store", category.Store)
	categoriesGroup.Get("/:id/edit", category.Edit)
	categoriesGroup.Put("/:id/update", category.Update)
	// categoriesGroup.Delete("/:id/delete", category.Delete)

	productsGroup := app.Group("/products")
	productsGroup.Get("/anyData", product.AnyData)
	productsGroup.Get("/remain-anyData", product.RemainAnyData)
	productsGroup.Post("/store", product.Store)
	productsGroup.Get("/:id/edit", product.Edit)
	productsGroup.Put("/:id/update", product.Update)

	saleReport := app.Group("/sale-reports")
	saleReport.Get("/daily", saleofreport.GetDailySalesReport) // ดึงรายงานแบบวัน
	saleReport.Get("/monthly", saleofreport.GetMonthlySalesReport)
	saleReport.Get("/:uuid/daily", saleofreport.GetDailySalesReportDetail)

	saleProductReport := app.Group("/sale-product-reports")
	saleProductReport.Get("/daily", saleofreport.GetDailySaleProductReport) // ดึงรายงานแบบวัน
	saleProductReport.Get("/monthly", saleofreport.GetMonthlySaleProductReport)
	// saleReport.Get("/:uuid/daily/edit", saleofreport.GetDailySalesReportDetailEdit)
	// point-of-sales/visitation/${tableIndex}/time/${minutes}
	// pointofsalesGroup.Post("/:id/:time")

	settingSystem := app.Group("/setting-systems")
	settingSystem.Put("/update", settingsystem.SaveSettingSystem)
	settingSystem.Get("/:id/data", settingsystem.GetSettingSystem)

	suppliers := app.Group("/suppliers")
	suppliers.Get("/anyData", supplier.AnyData)
	suppliers.Post("/store", supplier.Store)
	suppliers.Get("/:id/edit", supplier.Edit)
	suppliers.Put("/:id/update", supplier.Update)

	receipts := app.Group("/receipts")
	receipts.Post("/submit", receipt.SubmitReceipt)
	receipts.Post("/finalize", receipt.FinalizeReceipt)
	receipts.Get("/draft", receipt.DraftReceipt)
	receipts.Get("/:id/edit", receipt.EditReceipt)
	receipts.Delete("/:id/delete", receipt.DeleteReceipt)

	//	usersGroup := app.Group("/users", checkJWTCookie, middleware.PermissionMiddleware("users-access"))
	productReceiptReport := app.Group("/product-receipt-reports", checkJWTHeader, middleware.PermissionMiddleware("product-receipt-reports-access"))
	productReceiptReport.Get("/anyData", receiptreport.AnyData)
	productReceiptReport.Get("/:id/edit", receiptreport.EditView, checkJWTHeader, middleware.PermissionMiddleware("product-receipt-reports-edit"))
	productReceiptReport.Put("/:id/update", receiptreport.Update, checkJWTHeader, middleware.PermissionMiddleware("product-receipt-reports-edit"))
	productReceiptReport.Delete("/:id/delete", receiptreport.Delete, checkJWTHeader, middleware.PermissionMiddleware("product-receipt-reports-delete"))
	productReceiptReportAPI := productReceiptReport.Group("/api", checkJWTHeader, middleware.PermissionMiddleware("product-receipt-reports-edit"))
	productReceiptReportAPI.Put("/supplier/:id/update", receiptreport.SupplierUpdate)
	productReceiptReportAPI.Post("/draft/:id/submit", receiptreport.SubmitDraft)

	productTransactionReport := app.Group("/product-transaction-reports", checkJWTHeader, middleware.PermissionMiddleware("product-transactions-access"))
	productTransactionReportAPI := productTransactionReport.Group("/api", checkJWTHeader, middleware.PermissionMiddleware("product-transactions-access"))
	productTransactionReportAPI.Get("/search", transactionreport.SearchProductTransactions)
	productTransactionReportAPI.Get("/products", transactionreport.ListProducts)

	// setting-point-of-sale-access

	settingPointOfSale := app.Group("/setting-point-of-sales", checkJWTHeader, middleware.PermissionMiddleware("setting-point-of-sale-access"))
	settingPointOfSale.Get("", settingpointofsale.GetView)
	// settingPointOfSale.Get("/data", settingpointofsale.GetSettingPointOfSale)
	settingPointOfSale.Put("/update", settingpointofsale.SaveSettingPointOfSale)
	// settingSystem.Put("/update", settingsystem.SaveSettingSystem)
	// settingSystem.Get("/:id/data", settingsystem.GetSettingSystem)

	// productReceiptReport := app.Group("/product-receipt-reports", checkJWTCookie, middleware.PermissionMiddleware("product-receipt-reports-access"))
	// productReceiptReport.Get("/anyData", receiptreport.AnyData)
	// productReceiptReport.Get("/:id/edit", receiptreport.EditView, checkJWTCookie, middleware.PermissionMiddleware("product-receipt-reports-edit"))
	// productReceiptReport.Put("/:id/update", receiptreport.Update, checkJWTCookie, middleware.PermissionMiddleware("product-receipt-reports-edit"))
	// productReceiptReport.Delete("/:id/delete", receiptreport.Delete, checkJWTCookie, middleware.PermissionMiddleware("product-receipt-reports-delete"))
	// productReceiptReportAPI := productReceiptReport.Group("/api", checkJWTCookie, middleware.PermissionMiddleware("product-receipt-reports-edit"))
	// productReceiptReportAPI.Put("/supplier/:id/update", receiptreport.SupplierUpdate)
	// productReceiptReportAPI.Post("/draft/:id/submit", receiptreport.SubmitDraft)

	// productTransactionReport := app.Group("/product-transaction-reports", checkJWTCookie, middleware.PermissionMiddleware("product-transactions-access"))
	// productTransactionReportAPI := productTransactionReport.Group("/api", checkJWTCookie, middleware.PermissionMiddleware("product-transactions-access"))
	// productTransactionReportAPI.Get("/search", transactionreport.SearchProductTransactions)
	// productTransactionReportAPI.Get("/products", transactionreport.ListProducts)

	app.Post("/activate", license.ActivateLicense)
	app.Get("/license-status", license.CheckLicenseStatus)
	app.Get("/machine-id", license.GetMachineID)

}
