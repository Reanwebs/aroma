package app

import (
	"cardamom-dryer/internal/config"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NewRouter builds the HTTP API. It is shared by the long-running server and
// the Netlify function adapter so both deployment modes expose the same routes.
func NewRouter(db *gorm.DB, cfg config.Config) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:")
	})
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	api := NewAPI(db, cfg)
	v := r.Group("/api/v1")
	v.GET("/whatsapp/webhook", api.WhatsAppWebhook)
	v.POST("/whatsapp/webhook", api.WhatsAppWebhook)
	v.POST("/auth/login", api.Login)
	v.POST("/auth/refresh", api.Refresh)
	secure := v.Group("", api.Auth)
	secure.GET("/auth/me", api.Me)
	secure.POST("/customers", roles("SUPER_ADMIN", "ADMIN", "OPERATOR"), api.CreateCustomer)
	secure.PATCH("/customers/:id", roles("SUPER_ADMIN", "ADMIN", "OPERATOR"), api.UpdateCustomer)
	secure.GET("/customers", api.ListCustomers)
	secure.GET("/customers/:id/photo", api.CustomerPhoto)
	secure.GET("/customers/:id", api.GetCustomer)
	secure.GET("/customers/:id/history", api.CustomerHistory)
	secure.POST("/jobs", roles("SUPER_ADMIN", "ADMIN", "OPERATOR"), api.CreateJob)
	secure.GET("/jobs", api.ListJobs)
	secure.GET("/jobs/:id", api.GetJob)
	secure.PATCH("/jobs/:id", roles("SUPER_ADMIN", "ADMIN", "OPERATOR"), api.UpdateJob)
	secure.DELETE("/jobs/:id", roles("SUPER_ADMIN", "ADMIN", "OPERATOR"), api.DeleteJob)
	secure.POST("/jobs/:id/status", roles("SUPER_ADMIN", "ADMIN", "OPERATOR"), api.Status)
	secure.POST("/jobs/:id/weights", roles("SUPER_ADMIN", "ADMIN", "OPERATOR"), api.Weight)
	secure.POST("/jobs/:id/charges", roles("SUPER_ADMIN", "ADMIN"), api.Charge)
	secure.POST("/jobs/:id/invoice", roles("SUPER_ADMIN", "ADMIN", "ACCOUNTANT"), api.Invoice)
	secure.GET("/invoices", api.ListInvoices)
	secure.GET("/invoices/:id", api.GetInvoice)
	secure.GET("/invoices/:id/text", api.InvoiceText)
	secure.POST("/invoices/:id/payments", roles("SUPER_ADMIN", "ADMIN", "ACCOUNTANT"), api.Payment)
	secure.GET("/dashboard/summary", api.Dashboard)
	secure.GET("/analytics", api.Analytics)
	secure.GET("/market/auction-prices", api.AuctionPrices)
	return r
}
