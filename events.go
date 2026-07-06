package sdk

// Event names published by the Tabibu billing module.
const (
	EventPaymentRequested = "billing.payment_requested"
	EventBillCancelled    = "billing.bill_cancelled"
)

// PaymentRequestedPayload is the payload for billing.payment_requested events.
// Published when a bill is ready for collection via an external payment provider.
type PaymentRequestedPayload struct {
	BillID      string  `json:"bill_id"`
	PatientID   string  `json:"patient_id"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	Method      string  `json:"method"`       // mpesa, card, insurance, etc.
	PhoneNumber string  `json:"phone_number"` // for mobile money
	Reference   string  `json:"reference"`    // optional (e.g. SHA member number)
}

// BillCancelledPayload is the payload for billing.bill_cancelled events.
// Published when a bill is cancelled. Any in-flight payment requests (e.g. a
// pending STK Push) should be abandoned.
type BillCancelledPayload struct {
	BillID string `json:"bill_id"`
}
