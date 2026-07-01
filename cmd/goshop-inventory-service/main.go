package main

import (
	"GoShop/handlers"
	"GoShop/internal/app"
)

func main() {
	app.RunService(app.ServiceOptions{
		Name:        "goshop-inventory-service",
		DefaultPort: app.EnvInt("GOSHOP_INVENTORY_PORT", 8103),
		SeedData:    true,
		Register:    handlers.RegisterInventoryServiceRoutes,
	})
}
