package main

import (
	"GoShop/handlers"
	"GoShop/internal/app"
)

func main() {
	app.RunService(app.ServiceOptions{
		Name:        "goshop-scheduler-service",
		DefaultPort: app.EnvInt("GOSHOP_SCHEDULER_PORT", 8109),
		SeedData:    true,
		Background:  handlers.StartReliableDelayQueueWorker,
	})
}
