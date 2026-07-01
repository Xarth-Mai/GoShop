package main

import (
	"GoShop/handlers"
	"GoShop/internal/app"
)

func main() {
	app.RunService(app.ServiceOptions{
		Name:        "goshop-user-service",
		DefaultPort: app.EnvInt("GOSHOP_USER_PORT", 8101),
		SeedData:    true,
		Register:    handlers.RegisterUserServiceRoutes,
	})
}
