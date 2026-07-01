package main

import (
	"GoShop/handlers"
	"GoShop/internal/app"
)

func main() {
	app.RunService(app.ServiceOptions{
		Name:        "goshop-payment-service",
		DefaultPort: app.EnvInt("GOSHOP_PAYMENT_PORT", 8106),
		SeedData:    true,
		Register:    handlers.RegisterPaymentServiceRoutes,
	})
}
