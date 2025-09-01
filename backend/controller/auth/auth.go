package auth

import (
	"fmt"
	"log"

	"github.com/rainza999/fiber-test/db"
	"gorm.io/gorm"

	"time"

	// "github.com/dgrijalva/jwt-go"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	model "github.com/rainza999/fiber-test/models"
	"golang.org/x/crypto/bcrypt"

	helper "github.com/rainza999/fiber-test/controller/helper"
)

var jwtSecret = []byte("my-secret-key")

// Binding from JSON
type LoginBody struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}
type CustomClaims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	RoleID   uint   `json:"role_id"`
	// ExpiresAt int64  `json:"exp"`
	jwt.StandardClaims
}

func Login(c *fiber.Ctx) error {
	var json LoginBody
	if err := c.BodyParser(&json); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// ดึง user
	var userExists model.User
	if err := db.Db.Where("username = ?", json.Username).First(&userExists).Error; err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "Invalid username or password"})
	}

	// เช็ค password
	if !CheckPasswordHash(json.Password, userExists.Password) {
		return c.Status(401).JSON(fiber.Map{"error": "Invalid username or password"})
	}

	// สร้าง JWT
	claims := CustomClaims{
		UserID:   userExists.ID,
		Username: userExists.Username,
		RoleID:   userExists.RoleID,
		// ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate token"})
	}

	// ✅ ส่ง token กลับไป ไม่ใช้ cookie
	menus, err := helper.GetMenusByRole(userExists.RoleID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Menu load error"})
	}

	return c.JSON(fiber.Map{
		"status": "ok",
		"token":  token,
		"menus":  menus,
	})
}

func RefreshToken(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"message": "Refresh token route"})
}

// // func Login(c *fiber.Ctx) error {
// func Login(c *fiber.Ctx) error {
// 	var json LoginBody

// 	// Parse request body
// 	if err := c.BodyParser(&json); err != nil {
// 		return c.Status(400).JSON(fiber.Map{
// 			"error": "Invalid request body",
// 		})
// 	}
// 	// พิมพ์ข้อมูล credentials ลงใน console
// 	fmt.Println("Received credentials:", json)
// 	// Check if user exists
// 	var userExists model.User
// 	if err := db.Db.Where("username = ?", json.Username).First(&userExists).Error; err != nil {
// 		return c.Status(401).JSON(fiber.Map{
// 			"error": "Invalid username or password1",
// 		})
// 	}
// 	println("password from frontend: ", json.Password)
// 	println("password from db: ", userExists.Password)
// 	// plainPassword := "1609411"
// 	// // ลองสร้าง hash ใหม่
// 	// newHash, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcrypt.DefaultCost)
// 	// if err != nil {
// 	// 	fmt.Println("Error generating hash:", err)

// 	// }
// 	// fmt.Println("Newly generated hash:", string(newHash))

// 	// Check password
// 	if !CheckPasswordHash(json.Password, userExists.Password) {
// 		println("Password mismatch")
// 		return c.Status(401).JSON(fiber.Map{
// 			"error": "Invalid username or password2",
// 		})
// 	}
// 	println("Password match")

// 	// claims := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.StandardClaims{
// 	// 	Issuer:    strconv.Itoa(int(userExists.ID)),
// 	// 	ExpiresAt: time.Now().Add(time.Hour * 24).Unix(), //1 day
// 	// })

// 	claims := jwt.NewWithClaims(jwt.SigningMethodHS256, CustomClaims{
// 		UserID:    userExists.ID,
// 		Username:  userExists.Username,
// 		RoleID:    userExists.RoleID,
// 		ExpiresAt: time.Now().Add(time.Hour * 24000).Unix(),
// 	})

// 	// Create JWT token
// 	// token := jwt.New(jwt.SigningMethodHS256)
// 	// claims := token.Claims.(jwt.MapClaims)
// 	// claims["user_id"] = userExists.ID
// 	// claims["exp"] = time.Now().Add(time.Hour * 72).Unix()

// 	token, err := claims.SignedString(jwtSecret)
// 	if err != nil {
// 		return c.SendStatus(fiber.StatusInternalServerError)
// 	}

// 	cookie := fiber.Cookie{
// 		Name:     "jwt",
// 		Value:    token,
// 		Expires:  time.Now().Add(time.Hour * 24000),
// 		HTTPOnly: true,
// 	}

// 	c.Cookie(&cookie)
// 	// Return the token to the client

// 	// var role model.Role
// 	// db.Db.Model(&userExists).Association("Role").Find(&role)

// 	// var roleHasPermissions []model.RoleHasPermission
// 	// db.Db.Model(&role).Association("RoleHasPermissions").Find(&roleHasPermissions)

// 	// var permissions []model.Permission

// 	// // ใช้ loop ในการดึงข้อมูล Permission จากแต่ละ RoleHasPermission
// 	// for _, rhp := range roleHasPermissions {
// 	// 	var permission model.Permission
// 	// 	db.Db.First(&permission, rhp.PermissionID)
// 	// 	permissions = append(permissions, permission)
// 	// }

// 	// // สร้าง slice เพื่อเก็บข้อมูล Menu
// 	// var menus []model.Menu

// 	// // ใช้ loop ในการดึงข้อมูล Menu จากแต่ละ Permission
// 	// for _, permission := range permissions {
// 	// 	var menu model.Menu
// 	// 	db.Db.First(&menu, permission.MenuID)
// 	// 	menus = append(menus, menu)
// 	// }
// 	// log.Println(role)
// 	// log.Println("Role")

// 	// log.Println(roleHasPermissions)
// 	// log.Println("roleHasPermissions")

// 	// log.Println(permissions)
// 	// log.Println("permissions")

// 	// log.Println(menus)
// 	// log.Println("menus")

// 	// log.Println("Login")
// 	// log.Println(userExists.RoleID)

// 	// redisKey := fmt.Sprintf("menus_%d", userExists.RoleID)

// 	// // ใช้คำสั่ง GET เพื่อดึงข้อมูลจาก Redis
// 	// cachedData, err := client.Get(ctx, redisKey).Result()

// 	// if err == redis.Nil {

// 	// } else if err != nil {
// 	// 	// เกิด error อื่น ๆ
// 	// 	return c.SendStatus(fiber.StatusInternalServerError)
// 	// }

// 	menus, err := helper.GetMenusByRole(userExists.RoleID)
// 	if err != nil {
// 		fmt.Println("Error fetching menus:", err)
// 	}
// 	fmt.Println("Menus:", menus)
// 	return c.Status(200).JSON(fiber.Map{
// 		"status": "ok",
// 		"token":  token,
// 		"menus":  menus,
// 		// "permissions": permissions,
// 	})

// }

// You might need to implement a password hashing function like bcrypt
func CheckPasswordHash(password, hash string) bool {
	println("password: ", password)
	println("hash: ", hash)
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
func HashPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedPassword), nil
}

// GenerateJWTToken generates a JWT token with expiration time
func GenerateJWTToken(fullname string) (string, error) {
	// Set expiration time to 24 hours
	expirationTime := time.Now().Add(24000 * time.Hour)

	// Create the JWT claims, which include the username and expiration time
	claims := &jwt.StandardClaims{
		Subject:   fullname,
		ExpiresAt: expirationTime.Unix(),
	}

	// Create the token with claims and sign it with the secret key
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

type User struct {
	gorm.Model
	Username string `gorm:"unique"`
	Password string
	RoleID   uint
}

func LoginUser(c *fiber.Ctx) error {
	var input User
	var user model.User

	if err := c.BodyParser(&input); err != nil {
		return err
	}
	println("abc")
	// Find user by email
	db.Db.Where("username = ?", input.Username).First(&user)

	// // Check password
	// if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password)); err != nil {
	// 	return c.SendStatus(fiber.StatusUnauthorized)
	// }

	if !CheckPasswordHash(input.Password, user.Password) {
		return c.Status(401).JSON(fiber.Map{
			"error": "Invalid username or password2",
		})
	}
	c.ClearCookie()
	// Create JWT token
	token := jwt.New(jwt.SigningMethodHS256)
	claims := token.Claims.(jwt.MapClaims)
	claims["user_id"] = user.ID
	claims["exp"] = time.Now().Add(time.Hour * 7200).Unix()

	fmt.Println(user.ID)
	t, err := token.SignedString([]byte("my-secret-key"))
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	// Set cookie
	c.Cookie(&fiber.Cookie{
		Name:     "jwt",
		Value:    t,
		Expires:  time.Now().Add(time.Hour * 7200),
		Domain:   "*",
		HTTPOnly: false,
		Secure:   false, // ใช้ false หากไม่ได้ใช้ HTTPS
		SameSite: "Lax",
	})
	log.Println("Login successful. JWT token:", t)
	log.Println("teset Login Show Cookie:", c.Cookies("jwt"))

	return c.JSON(fiber.Map{"message": "success"})
}

func Users(c *fiber.Ctx) error {
	cookie := c.Cookies("jwt")
	fmt.Println(cookie)
	fmt.Println("cookie")
	token, err := jwt.ParseWithClaims(cookie, &jwt.StandardClaims{}, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	if err != nil {
		c.Status(fiber.StatusUnauthorized)
		return c.JSON(fiber.Map{
			"message": "unauthenticated",
		})
	}

	claims := token.Claims.(*jwt.StandardClaims)

	var user model.User

	db.Db.Where("id = ?", claims.Issuer).First(&user)

	return c.JSON(user)
}
