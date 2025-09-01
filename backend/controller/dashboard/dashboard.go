package dashboard

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
)

func GetView(c *fiber.Ctx) error {

	return c.JSON(fiber.Map{
		"status": "ok",
	})
	// return c.Status(200).JSON(fiber.Map{
	// 	"status": "ok",
	// })
}

func AnyData(c *fiber.Ctx) error {

	fmt.Println("hello AnyData")
	var lists []model.Division

	result := db.Db.Find(&lists)

	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	}
	return c.JSON(lists)
}
