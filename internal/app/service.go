package app

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"strings"
	"time"
)

type Service struct{ db *gorm.DB }

func NewService(db *gorm.DB) *Service { return &Service{db} }
func (s *Service) CreateCustomer(ctx context.Context, c *Customer) (*Customer, error) {
	if c.Phone == "" || c.Name == "" {
		return nil, ErrInvalidState
	}
	var n int64
	s.db.WithContext(ctx).Model(&Customer{}).Count(&n)
	c.CustomerCode = fmt.Sprintf("CUS-%06d", n+1)
	if e := s.db.WithContext(ctx).Create(c).Error; e != nil {
		return nil, e
	}
	return c, nil
}
func (s *Service) Customers(ctx context.Context, search, phone string, page, limit int) ([]Customer, int64, error) {
	q := s.db.WithContext(ctx).Model(&Customer{})
	if search != "" {
		like := "%" + search + "%"
		q = q.Where("(name ILIKE ? OR phone ILIKE ? OR whatsapp_number ILIKE ?)", like, like, like)
	}
	if phone != "" {
		q = q.Where("phone = ?", phone)
	}
	var n int64
	q.Count(&n)
	var rows []Customer
	q = q.Select("customers.id, customers.customer_code, customers.name, customers.phone, customers.whatsapp_number, customers.address, customers.village, customers.district, customers.gst_number, customers.created_at, customers.updated_at, (customers.profile_image_data IS NOT NULL AND customers.profile_image_data <> '') AS has_profile_image")
	e := q.Order("created_at DESC").Offset((page - 1) * limit).Limit(limit).Find(&rows).Error
	return rows, n, e
}
func (s *Service) Customer(ctx context.Context, id uuid.UUID) (*Customer, error) {
	var c Customer
	if e := s.db.WithContext(ctx).First(&c, "id = ?", id).Error; e != nil {
		return nil, ErrNotFound
	}
	return &c, nil
}
func (s *Service) UpdateCustomer(ctx context.Context, id uuid.UUID, c *Customer) (*Customer, error) {
	if strings.TrimSpace(c.Name) == "" || strings.TrimSpace(c.Phone) == "" {
		return nil, ErrInvalidState
	}
	updates := map[string]any{
		"name": strings.TrimSpace(c.Name), "phone": strings.TrimSpace(c.Phone),
		"whatsapp_number": strings.TrimSpace(c.WhatsAppNumber), "address": strings.TrimSpace(c.Address),
		"village": strings.TrimSpace(c.Village), "district": strings.TrimSpace(c.District),
		"gst_number": strings.TrimSpace(c.GSTNumber), "profile_image_data": c.ProfileImageData,
		"updated_at": time.Now().UTC(),
	}
	result := s.db.WithContext(ctx).Model(&Customer{}).Where("id=?", id).Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return s.Customer(ctx, id)
}
func (s *Service) CreateJob(ctx context.Context, customerID, userID uuid.UUID, product string, weight, price decimal.Decimal, bagCount int, notes string) (*DryingJob, error) {
	if weight.LessThanOrEqual(decimal.Zero) || price.LessThanOrEqual(decimal.Zero) || bagCount < 1 {
		return nil, ErrInvalidState
	}
	var out DryingJob
	e := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var c Customer
		if e := tx.First(&c, "id=?", customerID).Error; e != nil {
			return ErrNotFound
		}
		var n int64
		tx.Model(&DryingJob{}).Where("EXTRACT(YEAR FROM received_at)=?", time.Now().Year()).Count(&n)
		now := time.Now().UTC()
		out = DryingJob{Number: fmt.Sprintf("JOB-%d-%06d", now.Year(), n+1), CustomerID: customerID, ProductType: product, BagCount: bagCount, Status: "RECEIVED", ReceivedAt: now, Notes: notes, CreatedBy: &userID}
		if out.ProductType == "" {
			out.ProductType = "CARDAMOM"
		}
		if e := tx.Create(&out).Error; e != nil {
			return e
		}
		m := WeightMeasurement{DryingJobID: out.ID, MeasurementType: "RECEIVED", WeightKG: weight, MeasuredAt: now, MeasuredBy: &userID}
		if e := tx.Create(&m).Error; e != nil {
			return e
		}
		charge := Charge{DryingJobID: out.ID, ChargeType: "DRYING", Description: "Drying service", Quantity: weight, Unit: "KG", Rate: price, Amount: weight.Mul(price).Round(2)}
		if e := tx.Create(&charge).Error; e != nil {
			return e
		}
		return tx.Create(&StatusHistory{DryingJobID: out.ID, NewStatus: "RECEIVED", ChangedBy: &userID}).Error
	})
	if e != nil {
		return nil, e
	}
	return &out, nil
}
func (s *Service) Jobs(ctx context.Context, status, number, customerSearch string, page, limit int) ([]DryingJob, int64, error) {
	q := s.db.WithContext(ctx).Model(&DryingJob{}).Preload("Customer").Preload("Measurements")
	if status != "" {
		q = q.Where("status=?", status)
	}
	if number != "" || customerSearch != "" {
		q = q.Joins("JOIN customers ON customers.id = drying_jobs.customer_id")
		like := "%" + customerSearch + "%"
		if customerSearch == "" {
			like = "%" + number + "%"
		}
		if number != "" && customerSearch != "" {
			q = q.Where("(drying_jobs.job_number ILIKE ? OR customers.name ILIKE ? OR customers.phone ILIKE ? OR customers.whatsapp_number ILIKE ?)", "%"+number+"%", like, like, like)
		} else if customerSearch != "" {
			q = q.Where("(customers.name ILIKE ? OR customers.phone ILIKE ? OR customers.whatsapp_number ILIKE ?)", like, like, like)
		} else {
			q = q.Where("drying_jobs.job_number ILIKE ?", like)
		}
	}
	var n int64
	q.Count(&n)
	var v []DryingJob
	e := q.Order("received_at DESC").Offset((page - 1) * limit).Limit(limit).Find(&v).Error
	return v, n, e
}
func (s *Service) Job(ctx context.Context, id uuid.UUID) (*DryingJob, error) {
	var j DryingJob
	e := s.db.WithContext(ctx).Preload("Customer").Preload("Measurements").Preload("Charges").First(&j, "id=?", id).Error
	if e != nil {
		return nil, ErrNotFound
	}
	return &j, nil
}
func (s *Service) ChangeStatus(ctx context.Context, id, user uuid.UUID, to, reason string) (*DryingJob, error) {
	var j DryingJob
	e := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.First(&j, "id=?", id).Error; e != nil {
			return ErrNotFound
		}
		if !validTransition(j.Status, to) {
			return ErrInvalidTransition
		}
		old := j.Status
		j.Status = to
		if to == "COMPLETED" {
			n := time.Now().UTC()
			j.CompletedAt = &n
		}
		if e := tx.Save(&j).Error; e != nil {
			return e
		}
		if e := tx.Create(&StatusHistory{DryingJobID: id, OldStatus: &old, NewStatus: to, ChangedBy: &user, Reason: reason}).Error; e != nil {
			return e
		}
		if to == "COMPLETED" {
			return s.createInvoiceTx(tx, &j, taxZero(), taxZero())
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	return &j, nil
}
func (s *Service) AddMeasurement(ctx context.Context, id, user uuid.UUID, m *WeightMeasurement) error {
	if m.WeightKG.LessThanOrEqual(decimal.Zero) || (m.MoisturePercentage != nil && (m.MoisturePercentage.IsNegative() || m.MoisturePercentage.GreaterThan(decimal.NewFromInt(100)))) {
		return ErrInvalidState
	}
	var j DryingJob
	if e := s.db.WithContext(ctx).First(&j, "id=?", id).Error; e != nil {
		return ErrNotFound
	}
	if m.MeasurementType == "FINAL" && j.Status == "COMPLETED" {
		return ErrInvalidState
	}
	m.ID = uuid.Nil
	m.DryingJobID = id
	m.MeasuredBy = &user
	if m.MeasuredAt.IsZero() {
		m.MeasuredAt = time.Now().UTC()
	}
	return s.db.WithContext(ctx).Create(m).Error
}
func (s *Service) AddCharge(ctx context.Context, id uuid.UUID, c *Charge) error {
	if c.Quantity.IsNegative() || c.Rate.IsNegative() {
		return ErrInvalidState
	}
	var j DryingJob
	if s.db.WithContext(ctx).First(&j, "id=?", id).Error != nil {
		return ErrNotFound
	}
	c.ID = uuid.Nil
	c.DryingJobID = id
	c.Amount = c.Quantity.Mul(c.Rate).Round(2)
	return s.db.WithContext(ctx).Create(c).Error
}
func (s *Service) CreateInvoice(ctx context.Context, id, user uuid.UUID, tax, discount decimal.Decimal) (*Invoice, error) {
	var inv Invoice
	e := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing Invoice
		if tx.Where("drying_job_id=?", id).First(&existing).Error == nil {
			inv = existing
			return nil
		}
		var j DryingJob
		if tx.First(&j, "id=?", id).Error != nil {
			return ErrNotFound
		}
		if j.Status != "COMPLETED" {
			return ErrInvalidState
		}
		return s.createInvoiceTx(tx, &j, tax, discount)
	})
	if e != nil {
		return nil, e
	}
	if e := s.db.WithContext(ctx).Where("drying_job_id=?", id).First(&inv).Error; e != nil {
		return nil, e
	}
	return &inv, nil
}
func taxZero() decimal.Decimal { return decimal.Zero }
func (s *Service) createInvoiceTx(tx *gorm.DB, j *DryingJob, tax, discount decimal.Decimal) error {
	var inv Invoice
	if tx.Where("drying_job_id=?", j.ID).First(&inv).Error == nil {
		return nil
	}
	var final WeightMeasurement
	if tx.Where("drying_job_id=? AND measurement_type='FINAL'", j.ID).Order("measured_at DESC").First(&final).Error != nil {
		return ErrFinalWeightNeeded
	}
	var cs []Charge
	if e := tx.Where("drying_job_id=?", j.ID).Find(&cs).Error; e != nil {
		return e
	}
	if len(cs) == 0 {
		return ErrChargeNeeded
	}
	sub := decimal.Zero
	items := make([]InvoiceItem, 0, len(cs))
	for _, c := range cs {
		sub = sub.Add(c.Amount)
		items = append(items, InvoiceItem{Description: c.Description, Quantity: c.Quantity, Unit: c.Unit, Rate: c.Rate, Amount: c.Amount})
	}
	var n int64
	tx.Model(&Invoice{}).Where("EXTRACT(YEAR FROM issued_at)=?", time.Now().Year()).Count(&n)
	inv = Invoice{InvoiceNumber: fmt.Sprintf("INV-%d-%06d", time.Now().Year(), n+1), DryingJobID: j.ID, CustomerID: j.CustomerID, Subtotal: sub, Discount: discount, TaxAmount: tax, TotalAmount: sub.Sub(discount).Add(tax), Status: "ISSUED", IssuedAt: time.Now().UTC()}
	if inv.TotalAmount.IsNegative() {
		return ErrInvalidState
	}
	if e := tx.Create(&inv).Error; e != nil {
		return e
	}
	for i := range items {
		items[i].InvoiceID = inv.ID
		if e := tx.Create(&items[i]).Error; e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) RecordPayment(ctx context.Context, invoiceID, user uuid.UUID, p *Payment) (*Invoice, error) {
	var inv Invoice
	e := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Preload("Payments").First(&inv, "id=?", invoiceID).Error; e != nil {
			return ErrNotFound
		}
		if inv.Status == "CANCELLED" || inv.Status == "PAID" {
			return ErrInvalidState
		}
		paid := decimal.Zero
		for _, x := range inv.Payments {
			paid = paid.Add(x.Amount)
		}
		if p.Amount.LessThanOrEqual(decimal.Zero) || p.Amount.GreaterThan(inv.TotalAmount.Sub(paid)) {
			return ErrOverpayment
		}
		p.ID = uuid.Nil
		p.InvoiceID = invoiceID
		p.ReceivedBy = &user
		if p.TransactionReference != nil {
			reference := strings.TrimSpace(*p.TransactionReference)
			if reference == "" {
				p.TransactionReference = nil
			} else {
				p.TransactionReference = &reference
			}
		}
		if p.PaidAt.IsZero() {
			p.PaidAt = time.Now().UTC()
		}
		if e := tx.Create(p).Error; e != nil {
			return e
		}
		if paid.Add(p.Amount).Equal(inv.TotalAmount) {
			inv.Status = "PAID"
		} else {
			inv.Status = "PARTIALLY_PAID"
		}
		if e := tx.Save(&inv).Error; e != nil {
			return e
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	return &inv, nil
}
