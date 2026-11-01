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
	r := gin.New()
	r.Use(gin.Recovery(), func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:")
	})
	api := app.NewAPI(db, cfg)
	r.GET("/", func(c *gin.Context) { c.File("web/index.html") })
	r.Static("/web", "web")
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	v := r.Group("/api/v1")
	v.POST("/auth/login", api.Login)
	v.POST("/auth/refresh", api.Refresh)
	secure := v.Group("", api.Auth)
	secure.GET("/auth/me", api.Me)
	secure.POST("/customers", appRoles("SUPER_ADMIN", "ADMIN", "OPERATOR"), api.CreateCustomer)
	secure.PATCH("/customers/:id", appRoles("SUPER_ADMIN", "ADMIN", "OPERATOR"), api.UpdateCustomer)
	secure.GET("/customers", api.ListCustomers)
	secure.GET("/customers/:id/photo", api.CustomerPhoto)
	secure.GET("/customers/:id", api.GetCustomer)
	secure.GET("/customers/:id/history", api.CustomerHistory)
	secure.POST("/jobs", appRoles("SUPER_ADMIN", "ADMIN", "OPERATOR"), api.CreateJob)
	secure.GET("/jobs", api.ListJobs)
	secure.GET("/jobs/:id", api.GetJob)
	secure.POST("/jobs/:id/status", appRoles("SUPER_ADMIN", "ADMIN", "OPERATOR"), api.Status)
	secure.POST("/jobs/:id/weights", appRoles("SUPER_ADMIN", "ADMIN", "OPERATOR"), api.Weight)
	secure.POST("/jobs/:id/charges", appRoles("SUPER_ADMIN", "ADMIN"), api.Charge)
	secure.POST("/jobs/:id/invoice", appRoles("SUPER_ADMIN", "ADMIN", "ACCOUNTANT"), api.Invoice)
	secure.GET("/invoices", api.ListInvoices)
	secure.GET("/invoices/:id", api.GetInvoice)
	secure.GET("/invoices/:id/text", api.InvoiceText)
	secure.POST("/invoices/:id/payments", appRoles("SUPER_ADMIN", "ADMIN", "ACCOUNTANT"), api.Payment)
	secure.GET("/dashboard/summary", api.Dashboard)
	secure.GET("/analytics", api.Analytics)
	secure.GET("/market/auction-prices", api.AuctionPrices)
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
func appRoles(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		r, _ := c.Get("role")
		for _, x := range roles {
			if x == r {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(403, gin.H{"success": false, "error": gin.H{"code": "FORBIDDEN", "message": "insufficient permissions"}})
	}
}
