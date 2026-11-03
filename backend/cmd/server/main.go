package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"magu/internal/auth"
	"magu/internal/catalog"
	"magu/internal/consignment"
	"magu/internal/drive"
	"magu/internal/printer"
	"magu/internal/qris"
	"magu/internal/realtime"
	"magu/internal/sales"
	"magu/web"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server berhenti", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		return err
	}

	redisOpts, err := redis.ParseURL(os.Getenv("REDIS_URL"))
	if err != nil {
		return err
	}
	cache := redis.NewClient(redisOpts)
	defer cache.Close()
	if err := cache.Ping(ctx).Err(); err != nil {
		return err
	}

	app := fiber.New(fiber.Config{
		ErrorHandler:          handleError,
		BodyLimit:             16 << 20,
		DisableStartupMessage: true,
	})

	app.Get("/healthz", func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.UserContext(), 2*time.Second)
		defer cancel()
		if err := errors.Join(db.Ping(ctx), cache.Ping(ctx).Err()); err != nil {
			slog.Error("healthz gagal", "error", err)
			return fiber.ErrServiceUnavailable
		}
		return c.JSON(fiber.Map{"status": "ok"})
	})

	authHandler := auth.NewHandler(auth.NewService(db, cache))
	app.Post("/api/login", limiter.New(limiter.Config{
		Max:        10,
		Expiration: time.Minute,
		LimitReached: func(*fiber.Ctx) error {
			return fiber.NewError(fiber.StatusTooManyRequests, "Terlalu banyak percobaan masuk. Coba lagi sebentar.")
		},
	}), authHandler.Login)

	api := app.Group("/api", authHandler.Require)
	api.Post("/logout", authHandler.Logout)
	api.Get("/me", authHandler.Me)
	authHandler.UserRoutes(api)

	driveClient := drive.NewClient(drive.Config{
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		RefreshToken: os.Getenv("GOOGLE_REFRESH_TOKEN"),
		RootFolder:   os.Getenv("DRIVE_ROOT_FOLDER_ID"),
	})
	consignment.NewHandler(consignment.NewService(db, driveClient), envOr("STORE_NAME", "Toko")).Routes(api, auth.RequireAdmin)
	catalog.NewHandler(catalog.NewService(db, cache, envOr("STORE_CODE", "TK"))).Routes(api, auth.RequireAdmin)

	hub := realtime.NewHub()
	gateway := qris.NewClient(envOr("MIDTRANS_BASE_URL", "https://api.sandbox.midtrans.com"), os.Getenv("MIDTRANS_SERVER_KEY"))
	salesHandler := sales.NewHandler(sales.NewService(db), gateway, hub)
	salesHandler.Routes(api)
	app.Post("/webhooks/qris", salesHandler.Webhook)
	app.Get("/ws", authHandler.Require, func(c *fiber.Ctx) error {
		if !websocket.IsWebSocketUpgrade(c) {
			return fiber.ErrUpgradeRequired
		}
		return c.Next()
	}, hub.Handler())

	printerTarget := os.Getenv("PRINTER_TARGET")
	api.Post("/print", printer.Print(printerTarget))
	api.Get("/settings", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"store_name":    envOr("STORE_NAME", "Toko"),
			"receipt_width": envInt("RECEIPT_WIDTH", 32),
			"printer_ready": printerTarget != "",
			"qris_enabled":  gateway != nil,
			"drive_enabled": driveClient != nil,
		})
	})

	if dist, err := fs.Sub(web.Dist, "dist"); err == nil {
		if _, err := fs.Stat(dist, "index.html"); err == nil {
			app.Use(filesystem.New(filesystem.Config{Root: http.FS(dist)}))
		}
	}

	addr := envOr("LISTEN_ADDR", "127.0.0.1:8080")

	go func() {
		<-ctx.Done()
		app.ShutdownWithTimeout(10 * time.Second)
	}()

	slog.Info("server berjalan", "addr", addr)
	return app.Listen(addr)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return n
	}
	return fallback
}

func handleError(c *fiber.Ctx, err error) error {
	var fe *fiber.Error
	if errors.As(err, &fe) {
		return c.Status(fe.Code).JSON(fiber.Map{"error": fe.Message})
	}
	slog.Error("request gagal", "method", c.Method(), "path", c.Path(), "error", err)
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "terjadi kesalahan di server"})
}
