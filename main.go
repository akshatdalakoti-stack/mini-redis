package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store := NewStore()
	store.StartSweeper(ctx, time.Second)

	err := RunServer(ctx, store, ":6379")
	if err != nil {
		fmt.Println("server error:", err)
		os.Exit(1)
	}

	fmt.Println("mini-redis shut down")
}
