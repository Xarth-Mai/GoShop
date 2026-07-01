package main

import (
	"context"
	"time"

	"GoShop/core"
	"GoShop/handlers"
	"GoShop/internal/app"
	"GoShop/internal/outbox"
)

func main() {
	app.RunService(app.ServiceOptions{
		Name:        "goshop-scheduler-service",
		DefaultPort: app.EnvInt("GOSHOP_SCHEDULER_PORT", 8109),
		SeedData:    true,
		Background:  startBackgroundWorkers,
	})
}

func startBackgroundWorkers() {
	go outbox.NewPublisher(core.DB, core.Logger).Start(context.Background())
	time.Sleep(100 * time.Millisecond)
	handlers.StartReliableDelayQueueWorker()
}
