package main

import (
	"GoShop/handlers"
	"GoShop/internal/app"
)

func main() {
	app.RunService(app.ServiceOptions{
		Name:        "goshop-promotion-service",
		DefaultPort: app.EnvInt("GOSHOP_PROMOTION_PORT", 8104),
		SeedData:    true,
		Register:    handlers.RegisterPromotionServiceRoutes,
	})
}
