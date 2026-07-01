package main

import (
	"GoShop/handlers"
	"GoShop/internal/app"
)

func main() {
	app.RunService(app.ServiceOptions{
		Name:        "goshop-product-service",
		DefaultPort: app.EnvInt("GOSHOP_PRODUCT_PORT", 8102),
		SeedData:    true,
		Register:    handlers.RegisterProductServiceRoutes,
	})
}
