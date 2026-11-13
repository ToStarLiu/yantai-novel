package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"novel/internal/app"
	"novel/internal/server"
	"novel/migrations"
	"os"
	"os/signal"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	svc, err := app.Service(ctx)
	if err != nil {
		return err
	}
	defer svc.DB.Close()
	if len(os.Args) == 3 && os.Args[1] == "migrate" {
		return migrations.Apply(ctx, svc.DB, os.Args[2])
	}
	if err = migrations.Check(ctx, svc.DB); err != nil {
		return err
	}
	password := os.Getenv("AUTHOR_PASSWORD")
	if len(password) < 12 {
		return fmt.Errorf("AUTHOR_PASSWORD must contain at least 12 characters")
	}
	username := os.Getenv("AUTHOR_USERNAME")
	if username == "" {
		username = "author"
	}
	addr := os.Getenv("APP_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8088"
	}
	webRoot := os.Getenv("WEB_ROOT")
	if webRoot == "" {
		webRoot = "web/dist"
	}
	api := &server.Server{Service: svc, Auth: server.Auth{Username: username, Password: password, PublicOrigin: os.Getenv("PUBLIC_ORIGIN"), Secure: os.Getenv("COOKIE_SECURE") == "true"}, WebRoot: webRoot}
	httpServer := &http.Server{Addr: addr, Handler: api.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 1 << 20}
	workerErr := make(chan error, 1)
	if os.Getenv("RUN_WORKER") != "false" {
		go func() { workerErr <- svc.Worker(ctx) }()
	}
	serveErr := make(chan error, 1)
	go func() { fmt.Println("Novel workbench listening on", addr); serveErr <- httpServer.ListenAndServe() }()
	select {
	case <-ctx.Done():
	case err = <-workerErr:
		cancel()
	case err = <-serveErr:
		cancel()
	}
	shutdown, done := context.WithTimeout(context.Background(), 20*time.Second)
	defer done()
	_ = httpServer.Shutdown(shutdown)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
