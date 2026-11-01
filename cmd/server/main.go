package main

import (
	"cardamom-dryer/internal/app"
	"cardamom-dryer/internal/config"
	"cardamom-dryer/internal/database"
	"context"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg, e := config.Load()
	if e != nil {
		log.Fatal(e)
	}
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	db, e := database.Open(cfg.DatabaseDSN)
	if e != nil {
		logger.Fatal("database connection failed", zap.Error(e))
	}
	if os.Getenv("RUN_MIGRATIONS") != "false" {
		if e := database.Migrate(db); e != nil {
			logger.Fatal("migration failed", zap.Error(e))
		}
	}
	if cfg.AppEnv == "development" {
		if e := app.SeedDevelopment(db); e != nil {
			logger.Fatal("development seed failed", zap.Error(e))
		}
	}
	if cfg.AppEnv == "production" && cfg.InitialAdminEmail != "" && cfg.InitialAdminPassword != "" {
		if e := app.SeedInitialAdmin(db, cfg.InitialAdminName, cfg.InitialAdminEmail, cfg.InitialAdminPassword); e != nil {
			logger.Fatal("initial admin bootstrap failed", zap.Error(e))
		}
	}
	marketCtx, stopMarketScheduler := context.WithCancel(context.Background())
	app.StartDailyAuctionPriceScheduler(marketCtx, db, func(err error) { logger.Warn("daily auction price refresh failed", zap.Error(err)) })
	defer stopMarketScheduler()
	r := app.NewRouter(db, cfg)
	r.GET("/", func(c *gin.Context) { c.File("web/index.html") })
	r.Static("/web", "web")
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Info("server started", zap.String("port", cfg.Port))
		if e := srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			logger.Fatal("server failed", zap.Error(e))
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
