package app

import "errors"

var (
	ErrNotFound          = errors.New("not found")
	ErrConflict          = errors.New("conflict")
	ErrInvalidTransition = errors.New("invalid job status transition")
	ErrInvalidState      = errors.New("invalid business state")
	ErrFinalWeightNeeded = errors.New("enter the final measured weight before completing this job")
	ErrChargeNeeded      = errors.New("add at least one charge before completing this job")
	ErrOverpayment       = errors.New("payment exceeds outstanding balance")
)

func validTransition(from, to string) bool {
	m := map[string][]string{
		"RECEIVED":            {"COMPLETED", "DRYING", "WAITING_FOR_DRYING"},
		"WAITING_FOR_DRYING":  {"DRYING"},
		"DRYING":              {"COMPLETED", "DRYING_COMPLETED"},
		"DRYING_COMPLETED":    {"WEIGHT_VERIFICATION"},
		"WEIGHT_VERIFICATION": {"INVOICED"},
		"INVOICED":            {"COMPLETED"},
	}

	for _, v := range m[from] {
		if v == to {
			return true
		}
	}

	// Allow cancellation from any status except COMPLETED/INVOICED if required
	if to == "CANCELLED" && from != "COMPLETED" && from != "INVOICED" {
		return true
	}

	return false
}
