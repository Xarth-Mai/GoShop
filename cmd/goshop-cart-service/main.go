package main

import (
	"GoShop/handlers"
	"GoShop/internal/app"
)

func main() {
	app.RunService(app.ServiceOptions{
		Name:        "goshop-cart-service",
		DefaultPort: app.EnvInt("GOSHOP_CART_PORT", 8108),
		SeedData:    true,
		Register:    handlers.RegisterCartServiceRoutes,
	})
}
