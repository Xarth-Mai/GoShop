package main

import (
	"GoShop/handlers"
	"GoShop/internal/app"
)

func main() {
	app.RunService(app.ServiceOptions{
		Name:        "goshop-aftersale-service",
		DefaultPort: app.EnvInt("GOSHOP_AFTERSALE_PORT", 8107),
		SeedData:    true,
		Register:    handlers.RegisterAfterSaleServiceRoutes,
	})
}
