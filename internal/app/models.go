package app

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"time"
)

type User struct {
	ID                                     uuid.UUID `gorm:"type:uuid;default:gen_random_uuid()"`
	Name, Email, Phone, PasswordHash, Role string
	IsActive                               bool
	CreatedAt, UpdatedAt                   time.Time
}
type Customer struct {
	ID                                    uuid.UUID `gorm:"type:uuid;default:gen_random_uuid()"`
	CustomerCode, Name, Phone             string
	WhatsAppNumber                        string `gorm:"column:whatsapp_number"`
	ProfileImageData                      string `gorm:"column:profile_image_data" json:"-"`
	HasProfileImage                       bool   `gorm:"column:has_profile_image;->" json:"HasProfileImage"`
	Address, Village, District, GSTNumber string
	CreatedAt, UpdatedAt                  time.Time
	Jobs                                  []DryingJob
}
type DryingJob struct {
	ID                                uuid.UUID `gorm:"type:uuid;default:gen_random_uuid()"`
	JobNumber                         uuid.UUID `gorm:"-"`
	Number                            string    `gorm:"column:job_number"`
	CustomerID                        uuid.UUID
	Customer                          Customer
	ProductType, Status, Notes        string
	BagCount                          int `gorm:"column:bag_count"`
	ReceivedAt                        time.Time
	ExpectedCompletionAt, CompletedAt *time.Time
	CreatedBy                         *uuid.UUID
	CreatedAt, UpdatedAt              time.Time
	Measurements                      []WeightMeasurement
	Charges                           []Charge
}

func (DryingJob) TableName() string { return "drying_jobs" }

type WeightMeasurement struct {
	ID                 uuid.UUID `gorm:"type:uuid;default:gen_random_uuid()"`
	DryingJobID        uuid.UUID
	MeasurementType    string
	WeightKG           decimal.Decimal  `gorm:"type:numeric(12,3)"`
	MoisturePercentage *decimal.Decimal `gorm:"type:numeric(5,2)"`
	MeasuredAt         time.Time
	MeasuredBy         *uuid.UUID
	Notes              string
	CreatedAt          time.Time
}
type StatusHistory struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid()"`
	DryingJobID uuid.UUID
	OldStatus   *string
	NewStatus   string
	ChangedBy   *uuid.UUID
	Reason      string
	CreatedAt   time.Time
}

func (StatusHistory) TableName() string { return "job_status_history" }

type Charge struct {
	ID                            uuid.UUID `gorm:"type:uuid;default:gen_random_uuid()"`
	DryingJobID                   uuid.UUID
	ChargeType, Description, Unit string
	Quantity                      decimal.Decimal `gorm:"type:numeric(12,3)"`
	Rate, Amount                  decimal.Decimal `gorm:"type:numeric(12,2)"`
	CreatedAt                     time.Time
}
type Invoice struct {
	ID                                         uuid.UUID `gorm:"type:uuid;default:gen_random_uuid()"`
	InvoiceNumber                              string
	DryingJobID, CustomerID                    uuid.UUID
	Customer                                   Customer
	Subtotal, Discount, TaxAmount, TotalAmount decimal.Decimal `gorm:"type:numeric(12,2)"`
	Status                                     string
	IssuedAt                                   time.Time
	DueAt                                      *time.Time
	PDFURL                                     string
	CreatedAt, UpdatedAt                       time.Time
	Items                                      []InvoiceItem
	Payments                                   []Payment
}
type InvoiceItem struct {
	ID                uuid.UUID `gorm:"type:uuid;default:gen_random_uuid()"`
	InvoiceID         uuid.UUID
	Description, Unit string
	Quantity          decimal.Decimal `gorm:"type:numeric(12,3)"`
	Rate, Amount      decimal.Decimal `gorm:"type:numeric(12,2)"`
}
type Payment struct {
	ID                   uuid.UUID `gorm:"type:uuid;default:gen_random_uuid()"`
	InvoiceID            uuid.UUID
	Amount               decimal.Decimal `gorm:"type:numeric(12,2)"`
	PaymentMethod, Notes string
	TransactionReference *string
	PaidAt               time.Time
	ReceivedBy           *uuid.UUID
	CreatedAt            time.Time
}
type Batch struct {
	ID                                 uuid.UUID `gorm:"type:uuid;default:gen_random_uuid()"`
	BatchNumber, Status, Notes         string
	DryingStartedAt, DryingCompletedAt *time.Time
	CreatedAt, UpdatedAt               time.Time
}

func (Batch) TableName() string { return "drying_batches" }

type BatchJob struct {
	BatchID, DryingJobID uuid.UUID
	InputWeightKG        decimal.Decimal `gorm:"type:numeric(12,3)"`
}

func (BatchJob) TableName() string { return "drying_batch_jobs" }

type PricingRule struct {
	ID                                  uuid.UUID `gorm:"type:uuid;default:gen_random_uuid()"`
	Name, ChargeType, ProductType, Unit string
	Rate                                decimal.Decimal `gorm:"type:numeric(12,2)"`
	EffectiveFrom                       time.Time
	EffectiveTo                         *time.Time
	IsActive                            bool
	CreatedAt, UpdatedAt                time.Time
}
type Notification struct {
	ID                                                                                     uuid.UUID `gorm:"type:uuid;default:gen_random_uuid()"`
	CustomerID, DryingJobID, InvoiceID                                                     *uuid.UUID
	Channel, NotificationType, Recipient, Message, Status, ProviderMessageID, ErrorMessage string
	RetryCount                                                                             int
	SentAt, FailedAt                                                                       *time.Time
	CreatedAt                                                                              time.Time
}
