package app

import (
	"bytes"
	"cardamom-dryer/internal/config"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type API struct {
	svc      *Service
	db       *gorm.DB
	cfg      config.Config
	validate *validator.Validate
}

func NewAPI(db *gorm.DB, c config.Config) *API { return &API{NewService(db), db, c, validator.New()} }
func ok(c *gin.Context, status int, data any, msg string) {
	c.JSON(status, gin.H{"success": true, "data": data, "message": msg})
}
func fail(c *gin.Context, e error) {
	status, code := http.StatusBadRequest, "BAD_REQUEST"
	switch {
	case errors.Is(e, ErrNotFound):
		status, code = 404, "NOT_FOUND"
	case errors.Is(e, ErrConflict):
		status, code = 409, "CONFLICT"
	case errors.Is(e, ErrInvalidTransition), errors.Is(e, ErrInvalidState), errors.Is(e, ErrOverpayment):
		status, code = 422, "BUSINESS_RULE"
	}
	c.JSON(status, gin.H{"success": false, "error": gin.H{"code": code, "message": e.Error()}})
}
func page(c *gin.Context) (int, int) {
	p, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	l, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if p < 1 {
		p = 1
	}
	if l < 1 {
		l = 20
	}
	if l > 100 {
		l = 100
	}
	return p, l
}
func identity(c *gin.Context) (uuid.UUID, string) {
	return c.MustGet("user_id").(uuid.UUID), c.MustGet("role").(string)
}
func (a *API) Auth(c *gin.Context) {
	h := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	claims, e := parseToken(a.cfg.JWTSecret, h)
	if e != nil {
		fail(c, errors.New("unauthorized"))
		c.Abort()
		return
	}
	id, e := uuid.Parse(claims.Subject)
	if e != nil {
		fail(c, errors.New("unauthorized"))
		c.Abort()
		return
	}
	c.Set("user_id", id)
	c.Set("role", claims.Role)
	c.Next()
}
func roles(allowed ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, r := identity(c)
		for _, x := range allowed {
			if x == r {
				c.Next()
				return
			}
		}
		c.JSON(403, gin.H{"success": false, "error": gin.H{"code": "FORBIDDEN", "message": "insufficient permissions"}})
		c.Abort()
	}
}
func (a *API) Login(c *gin.Context) {
	var x struct {
		Email    string `json:"email" validate:"required,email"`
		Password string `json:"password" validate:"required"`
	}
	if c.ShouldBindJSON(&x) != nil || a.validate.Struct(x) != nil {
		fail(c, errors.New("invalid request"))
		return
	}
	var u User
	if a.db.Where("email=?", x.Email).First(&u).Error != nil || !u.IsActive || verify(u.PasswordHash, x.Password) != nil {
		c.JSON(401, gin.H{"success": false, "error": gin.H{"code": "INVALID_CREDENTIALS", "message": "invalid email or password"}})
		return
	}
	at, _ := token(a.cfg.JWTSecret, u.ID, u.Role, a.cfg.AccessTTL)
	rt, _ := token(a.cfg.JWTSecret, u.ID, u.Role, a.cfg.RefreshTTL)
	ok(c, 200, gin.H{"access_token": at, "refresh_token": rt, "expires_in": int(a.cfg.AccessTTL.Seconds())}, "logged in")
}
func (a *API) Refresh(c *gin.Context) {
	var x struct {
		RefreshToken string `json:"refresh_token"`
	}
	if c.ShouldBindJSON(&x) != nil {
		fail(c, errors.New("invalid request"))
		return
	}
	cl, e := parseToken(a.cfg.JWTSecret, x.RefreshToken)
	if e != nil {
		c.Status(401)
		return
	}
	id, _ := uuid.Parse(cl.Subject)
	at, e := token(a.cfg.JWTSecret, id, cl.Role, a.cfg.AccessTTL)
	if e != nil {
		fail(c, e)
		return
	}
	ok(c, 200, gin.H{"access_token": at, "expires_in": int(a.cfg.AccessTTL.Seconds())}, "refreshed")
}
func (a *API) Me(c *gin.Context) {
	id, _ := identity(c)
	var u User
	if a.db.First(&u, "id=?", id).Error != nil {
		fail(c, ErrNotFound)
		return
	}
	u.PasswordHash = ""
	ok(c, 200, u, "")
}
func (a *API) CreateCustomer(c *gin.Context) {
	var x struct {
		Name, Phone, WhatsAppNumber, Address, Village, District, GSTNumber string
		ProfileImage                                                       string `json:"profile_image"`
	}
	if c.ShouldBindJSON(&x) != nil {
		fail(c, errors.New("invalid request"))
		return
	}
	if err := validateProfileImage(x.ProfileImage); err != nil {
		fail(c, err)
		return
	}
	customer := Customer{Name: x.Name, Phone: x.Phone, WhatsAppNumber: x.WhatsAppNumber, Address: x.Address, Village: x.Village, District: x.District, GSTNumber: x.GSTNumber, ProfileImageData: x.ProfileImage}
	v, e := a.svc.CreateCustomer(c, &customer)
	if e != nil {
		fail(c, e)
		return
	}
	ok(c, 201, v, "customer created successfully")
}
func (a *API) UpdateCustomer(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		fail(c, ErrNotFound)
		return
	}
	var x struct {
		Name, Phone, WhatsAppNumber, Address, Village, District, GSTNumber string
		ProfileImage                                                       string `json:"profile_image"`
	}
	if c.ShouldBindJSON(&x) != nil {
		fail(c, errors.New("invalid request"))
		return
	}
	if err := validateProfileImage(x.ProfileImage); err != nil {
		fail(c, err)
		return
	}
	customer := Customer{Name: x.Name, Phone: x.Phone, WhatsAppNumber: x.WhatsAppNumber, Address: x.Address, Village: x.Village, District: x.District, GSTNumber: x.GSTNumber, ProfileImageData: x.ProfileImage}
	v, err := a.svc.UpdateCustomer(c, id, &customer)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, 200, gin.H{"customer": v, "profile_image": v.ProfileImageData}, "customer updated successfully")
}
func validateProfileImage(data string) error {
	if data == "" {
		return nil
	}
	parts := strings.SplitN(data, ",", 2)
	if len(parts) != 2 || !strings.HasSuffix(parts[0], ";base64") {
		return errors.New("profile image must be a PNG or JPEG image")
	}
	mimeType := strings.TrimSuffix(strings.TrimPrefix(parts[0], "data:"), ";base64")
	if mimeType != "image/png" && mimeType != "image/jpeg" {
		return errors.New("profile image must be a PNG or JPEG image")
	}
	decoded, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil || len(decoded) > 2*1024*1024 {
		return errors.New("profile image must be smaller than 2 MB")
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(decoded)); err != nil {
		return errors.New("profile image file is invalid")
	}
	return nil
}
func (a *API) ListCustomers(c *gin.Context) {
	p, l := page(c)
	v, n, e := a.svc.Customers(c, c.Query("search"), c.Query("phone"), p, l)
	if e != nil {
		fail(c, e)
		return
	}
	type customerListItem struct {
		Customer
		HasProfileImage bool `json:"HasProfileImage"`
	}
	items := make([]customerListItem, 0, len(v))
	for _, customer := range v {
		items = append(items, customerListItem{Customer: customer, HasProfileImage: customer.HasProfileImage})
	}
	ok(c, 200, gin.H{"items": items, "page": p, "limit": l, "total": n}, "")
}
func (a *API) GetCustomer(c *gin.Context) {
	id, e := uuid.Parse(c.Param("id"))
	if e != nil {
		fail(c, ErrNotFound)
		return
	}
	v, e := a.svc.Customer(c, id)
	if e != nil {
		fail(c, e)
		return
	}
	ok(c, 200, gin.H{"customer": v, "profile_image": v.ProfileImageData}, "")
}
func (a *API) CustomerPhoto(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		fail(c, ErrNotFound)
		return
	}
	customer, err := a.svc.Customer(c, id)
	if err != nil {
		fail(c, err)
		return
	}
	parts := strings.SplitN(customer.ProfileImageData, ",", 2)
	if len(parts) != 2 {
		fail(c, ErrNotFound)
		return
	}
	contentType := strings.TrimPrefix(strings.TrimSuffix(parts[0], ";base64"), "data:")
	data, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		fail(c, ErrNotFound)
		return
	}
	c.Header("Cache-Control", "private, max-age=300")
	c.Data(http.StatusOK, contentType, data)
}
func (a *API) CreateJob(c *gin.Context) {
	var x struct {
		CustomerID  uuid.UUID       `json:"customer_id"`
		ProductType string          `json:"product_type"`
		Notes       string          `json:"notes"`
		WeightKG    decimal.Decimal `json:"weight_kg"`
		PricePerKG  decimal.Decimal `json:"price_per_kg"`
	}
	if c.ShouldBindJSON(&x) != nil {
		fail(c, errors.New("invalid request"))
		return
	}
	u, _ := identity(c)
	v, e := a.svc.CreateJob(c, x.CustomerID, u, x.ProductType, x.WeightKG, x.PricePerKG, x.Notes)
	if e != nil {
		fail(c, e)
		return
	}
	ok(c, 201, v, "job created successfully")
}
func (a *API) ListJobs(c *gin.Context) {
	p, l := page(c)
	v, n, e := a.svc.Jobs(c, c.Query("status"), c.Query("job_number"), c.Query("customer_search"), p, l)
	if e != nil {
		fail(c, e)
		return
	}
	ok(c, 200, gin.H{"items": v, "page": p, "limit": l, "total": n}, "")
}
func (a *API) GetJob(c *gin.Context) {
	id, e := uuid.Parse(c.Param("id"))
	if e != nil {
		fail(c, ErrNotFound)
		return
	}
	v, e := a.svc.Job(c, id)
	if e != nil {
		fail(c, e)
		return
	}
	ok(c, 200, v, "")
}
func (a *API) Status(c *gin.Context) {
	id, e := uuid.Parse(c.Param("id"))
	var x struct {
		Status          string `json:"Status"`
		Reason          string `json:"Reason"`
		SendWhatsApp    bool   `json:"send_whatsapp"`
		WhatsAppMessage string `json:"whatsapp_message"`
	}
	if e != nil || c.ShouldBindJSON(&x) != nil {
		fail(c, errors.New("invalid request"))
		return
	}
	u, _ := identity(c)
	v, e := a.svc.ChangeStatus(c, id, u, x.Status, x.Reason)
	if e != nil {
		fail(c, e)
		return
	}
	result := gin.H{"job": v, "whatsapp_sent": false}
	if x.SendWhatsApp {
		customer, customerErr := a.svc.Customer(c, v.CustomerID)
		if customerErr != nil {
			result["whatsapp_error"] = "Could not load the customer's WhatsApp details."
		} else {
			if sendErr := a.sendWhatsApp(c.Request.Context(), customer.Phone, x.WhatsAppMessage); sendErr != nil {
				result["whatsapp_error"] = sendErr.Error()
			} else {
				result["whatsapp_sent"] = true
			}
		}
	}
	ok(c, 200, result, "status updated")
}
func (a *API) Weight(c *gin.Context) {
	id, e := uuid.Parse(c.Param("id"))
	var x WeightMeasurement
	if e != nil || c.ShouldBindJSON(&x) != nil {
		fail(c, errors.New("invalid request"))
		return
	}
	u, _ := identity(c)
	if e = a.svc.AddMeasurement(c, id, u, &x); e != nil {
		fail(c, e)
		return
	}
	ok(c, 201, x, "measurement recorded")
}
func (a *API) Charge(c *gin.Context) {
	id, e := uuid.Parse(c.Param("id"))
	var x Charge
	if e != nil || c.ShouldBindJSON(&x) != nil {
		fail(c, errors.New("invalid request"))
		return
	}
	if e = a.svc.AddCharge(c, id, &x); e != nil {
		fail(c, e)
		return
	}
	ok(c, 201, x, "charge added")
}
func (a *API) Invoice(c *gin.Context) {
	id, e := uuid.Parse(c.Param("id"))
	var x struct{ Tax, Discount decimal.Decimal }
	_ = c.ShouldBindJSON(&x)
	u, _ := identity(c)
	v, e := a.svc.CreateInvoice(c, id, u, x.Tax, x.Discount)
	if e != nil {
		fail(c, e)
		return
	}
	ok(c, 201, v, "invoice issued")
}
func (a *API) ListInvoices(c *gin.Context) {
	p, l := page(c)
	var v []Invoice
	q := a.db.Preload("Customer").Preload("Payments").Order("issued_at DESC")
	var n int64
	q.Model(&Invoice{}).Count(&n)
	if e := q.Offset((p - 1) * l).Limit(l).Find(&v).Error; e != nil {
		fail(c, e)
		return
	}
	ok(c, 200, gin.H{"items": v, "page": p, "limit": l, "total": n}, "")
}
func (a *API) GetInvoice(c *gin.Context) {
	id, e := uuid.Parse(c.Param("id"))
	if e != nil {
		fail(c, ErrNotFound)
		return
	}
	var v Invoice
	if e = a.db.Preload("Customer").Preload("Items").Preload("Payments").First(&v, "id=?", id).Error; e != nil {
		fail(c, ErrNotFound)
		return
	}
	ok(c, 200, v, "")
}
func (a *API) InvoiceText(c *gin.Context) {
	id, e := uuid.Parse(c.Param("id"))
	if e != nil {
		fail(c, ErrNotFound)
		return
	}
	var v Invoice
	if e = a.db.Preload("Customer").Preload("Items").First(&v, "id=?", id).Error; e != nil {
		fail(c, ErrNotFound)
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n%s\n\nINVOICE %s\nDate: %s\n\nCustomer: %s\nPhone: %s\n\n", a.cfg.BusinessName, a.cfg.BusinessAddress, v.InvoiceNumber, v.IssuedAt.Format("02-Jan-2006"), v.Customer.Name, v.Customer.Phone)
	for _, i := range v.Items {
		fmt.Fprintf(&b, "%-24s %s %s x %s = %s\n", i.Description, i.Quantity.String(), i.Unit, i.Rate.StringFixed(2), i.Amount.StringFixed(2))
	}
	fmt.Fprintf(&b, "\nSubtotal: %s\nDiscount: %s\nTax: %s\nTOTAL: %s\n", v.Subtotal.StringFixed(2), v.Discount.StringFixed(2), v.TaxAmount.StringFixed(2), v.TotalAmount.StringFixed(2))
	c.Data(200, "text/plain; charset=utf-8", []byte(b.String()))
}
func (a *API) CustomerHistory(c *gin.Context) {
	id, e := uuid.Parse(c.Param("id"))
	if e != nil {
		fail(c, ErrNotFound)
		return
	}
	v, e := a.svc.Customer(c, id)
	if e != nil {
		fail(c, e)
		return
	}
	var jobs []DryingJob
	var invoices []Invoice
	a.db.Where("customer_id=?", id).Order("received_at DESC").Find(&jobs)
	a.db.Preload("Payments").Where("customer_id=?", id).Find(&invoices)
	received, final, billed, paid := decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero
	for _, j := range jobs {
		var ms []WeightMeasurement
		a.db.Where("drying_job_id=?", j.ID).Find(&ms)
		for _, m := range ms {
			if m.MeasurementType == "RECEIVED" {
				received = received.Add(m.WeightKG)
			}
			if m.MeasurementType == "FINAL" {
				final = final.Add(m.WeightKG)
			}
		}
	}
	for _, i := range invoices {
		billed = billed.Add(i.TotalAmount)
		for _, p := range i.Payments {
			paid = paid.Add(p.Amount)
		}
	}
	ok(c, 200, gin.H{"customer": v, "jobs": jobs, "invoices": invoices, "total_received_kg": received, "total_final_kg": final, "total_billed": billed, "total_paid": paid, "outstanding": billed.Sub(paid)}, "")
}
func (a *API) Payment(c *gin.Context) {
	id, e := uuid.Parse(c.Param("id"))
	var x Payment
	if e != nil || c.ShouldBindJSON(&x) != nil {
		fail(c, errors.New("invalid request"))
		return
	}
	u, _ := identity(c)
	v, e := a.svc.RecordPayment(c, id, u, &x)
	if e != nil {
		fail(c, e)
		return
	}
	ok(c, 201, v, "payment recorded")
}
func (a *API) Dashboard(c *gin.Context) {
	start := time.Now().UTC().Truncate(24 * time.Hour)
	var jobs, drying, completed int64
	var receivedKG, dryingKG, completedKG decimal.Decimal
	a.db.Model(&DryingJob{}).Where("received_at>=?", start).Count(&jobs)
	a.db.Model(&DryingJob{}).Where("status='DRYING'").Count(&drying)
	a.db.Model(&DryingJob{}).Where("status='COMPLETED'").Count(&completed)
	a.db.Raw("SELECT COALESCE(SUM(w.weight_kg),0) FROM weight_measurements w JOIN drying_jobs j ON j.id=w.drying_job_id WHERE w.measurement_type='RECEIVED' AND j.received_at>=?", start).Scan(&receivedKG)
	a.db.Raw("SELECT COALESCE(SUM(w.weight_kg),0) FROM weight_measurements w JOIN drying_jobs j ON j.id=w.drying_job_id WHERE w.measurement_type='RECEIVED' AND j.status='DRYING'").Scan(&dryingKG)
	a.db.Raw("SELECT COALESCE(SUM(w.weight_kg),0) FROM drying_jobs j JOIN LATERAL (SELECT weight_kg FROM weight_measurements WHERE drying_job_id=j.id AND measurement_type='FINAL' ORDER BY measured_at DESC LIMIT 1) w ON TRUE WHERE j.status='COMPLETED'").Scan(&completedKG)
	ok(c, 200, gin.H{"jobs_received_today": jobs, "jobs_drying": drying, "jobs_completed": completed, "kg_received_today": receivedKG, "kg_drying": dryingKG, "kg_completed": completedKG}, "")
}

func (a *API) AuctionPrices(c *gin.Context) {
	feed, err := latestSavedAuctionPrices(c.Request.Context(), a.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "MARKET_PRICE_UNAVAILABLE", "message": "Saved auction prices are temporarily unavailable."}})
		return
	}
	ok(c, http.StatusOK, feed, "saved small-cardamom auction prices")
}

type analyticsPoint struct {
	Bucket         time.Time       `json:"bucket" gorm:"column:bucket"`
	JobsReceived   int64           `json:"jobs_received" gorm:"column:jobs_received"`
	JobsCompleted  int64           `json:"jobs_completed" gorm:"column:jobs_completed"`
	NewCustomers   int64           `json:"new_customers" gorm:"column:new_customers"`
	Invoices       int64           `json:"invoices" gorm:"column:invoices"`
	ReceivedKG     decimal.Decimal `json:"received_kg" gorm:"column:received_kg"`
	CompletedKG    decimal.Decimal `json:"completed_kg" gorm:"column:completed_kg"`
	BilledAmount   decimal.Decimal `json:"billed_amount" gorm:"column:billed_amount"`
	PaymentsAmount decimal.Decimal `json:"payments_amount" gorm:"column:payments_amount"`
}

func (a *API) Analytics(c *gin.Context) {
	from, err := time.Parse("2006-01-02", c.Query("from"))
	if err != nil {
		fail(c, errors.New("from must be a date in YYYY-MM-DD format"))
		return
	}
	to, err := time.Parse("2006-01-02", c.Query("to"))
	if err != nil {
		fail(c, errors.New("to must be a date in YYYY-MM-DD format"))
		return
	}
	if to.Before(from) {
		fail(c, errors.New("end date must be on or after start date"))
		return
	}
	end := to.AddDate(0, 0, 1)
	if end.Sub(from) > 2*366*24*time.Hour {
		fail(c, errors.New("choose a date range of two years or less"))
		return
	}
	days := end.Sub(from).Hours() / 24
	bucket := "day"
	if days > 45 && days <= 180 {
		bucket = "week"
	}
	if days > 180 {
		bucket = "month"
	}
	points := map[int64]*analyticsPoint{}
	get := func(t time.Time) *analyticsPoint {
		key := t.Unix()
		if points[key] == nil {
			points[key] = &analyticsPoint{Bucket: t}
		}
		return points[key]
	}
	var rows []analyticsPoint
	if err := a.db.Raw("SELECT date_trunc(?, j.received_at) bucket, COUNT(DISTINCT j.id) jobs_received, COALESCE(SUM(w.weight_kg),0) received_kg FROM drying_jobs j LEFT JOIN weight_measurements w ON w.drying_job_id=j.id AND w.measurement_type='RECEIVED' WHERE j.received_at>=? AND j.received_at<? GROUP BY bucket", bucket, from, end).Scan(&rows).Error; err != nil {
		fail(c, err)
		return
	}
	for _, p := range rows {
		x := get(p.Bucket)
		x.JobsReceived = p.JobsReceived
		x.ReceivedKG = p.ReceivedKG
	}
	rows = nil
	if err := a.db.Raw("SELECT date_trunc(?, j.completed_at) bucket, COUNT(*) jobs_completed, COALESCE(SUM(f.weight_kg),0) completed_kg FROM drying_jobs j LEFT JOIN LATERAL (SELECT weight_kg FROM weight_measurements WHERE drying_job_id=j.id AND measurement_type='FINAL' ORDER BY measured_at DESC LIMIT 1) f ON TRUE WHERE j.completed_at>=? AND j.completed_at<? GROUP BY bucket", bucket, from, end).Scan(&rows).Error; err != nil {
		fail(c, err)
		return
	}
	for _, p := range rows {
		x := get(p.Bucket)
		x.JobsCompleted = p.JobsCompleted
		x.CompletedKG = p.CompletedKG
	}
	rows = nil
	if err := a.db.Raw("SELECT date_trunc(?, created_at) bucket, COUNT(*) new_customers FROM customers WHERE created_at>=? AND created_at<? GROUP BY bucket", bucket, from, end).Scan(&rows).Error; err != nil {
		fail(c, err)
		return
	}
	for _, p := range rows {
		get(p.Bucket).NewCustomers = p.NewCustomers
	}
	rows = nil
	if err := a.db.Raw("SELECT date_trunc(?, issued_at) bucket, COUNT(*) invoices, COALESCE(SUM(total_amount),0) billed_amount FROM invoices WHERE issued_at>=? AND issued_at<? GROUP BY bucket", bucket, from, end).Scan(&rows).Error; err != nil {
		fail(c, err)
		return
	}
	for _, p := range rows {
		x := get(p.Bucket)
		x.Invoices = p.Invoices
		x.BilledAmount = p.BilledAmount
	}
	rows = nil
	if err := a.db.Raw("SELECT date_trunc(?, paid_at) bucket, COALESCE(SUM(amount),0) payments_amount FROM payments WHERE paid_at>=? AND paid_at<? GROUP BY bucket", bucket, from, end).Scan(&rows).Error; err != nil {
		fail(c, err)
		return
	}
	for _, p := range rows {
		get(p.Bucket).PaymentsAmount = p.PaymentsAmount
	}
	ordered := make([]analyticsPoint, 0, len(points))
	for _, p := range points {
		ordered = append(ordered, *p)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Bucket.Before(ordered[j].Bucket) })
	var totals analyticsPoint
	for _, p := range ordered {
		totals.JobsReceived += p.JobsReceived
		totals.JobsCompleted += p.JobsCompleted
		totals.NewCustomers += p.NewCustomers
		totals.Invoices += p.Invoices
		totals.ReceivedKG = totals.ReceivedKG.Add(p.ReceivedKG)
		totals.CompletedKG = totals.CompletedKG.Add(p.CompletedKG)
		totals.BilledAmount = totals.BilledAmount.Add(p.BilledAmount)
		totals.PaymentsAmount = totals.PaymentsAmount.Add(p.PaymentsAmount)
	}
	ok(c, 200, gin.H{"from": from.Format("2006-01-02"), "to": to.Format("2006-01-02"), "bucket": bucket, "points": ordered, "totals": totals}, "")
}
