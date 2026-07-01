package main

import (
	"GoShop/handlers"
	"GoShop/internal/app"
)

func main() {
	app.RunService(app.ServiceOptions{
		Name:        "goshop-order-service",
		DefaultPort: app.EnvInt("GOSHOP_ORDER_PORT", 8105),
		SeedData:    true,
		Register:    handlers.RegisterOrderServiceRoutes,
		Background:  handlers.RegisterOrderServiceSubscribers,
	})
}
