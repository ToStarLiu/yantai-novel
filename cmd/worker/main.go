package main

import (
	"context"
	"fmt"
	"novel/internal/app"
	"novel/migrations"
	"os"
	"os/signal"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	s, err := app.Service(ctx)
	if err == nil {
		defer s.DB.Close()
		err = migrations.Check(ctx, s.DB)
	}
	if err == nil {
		err = s.Worker(ctx)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
