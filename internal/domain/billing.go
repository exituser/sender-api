package domain

import "time"

type BillingSummary struct {
	Plan              Plan       `json:"plan"`
	Status            string     `json:"status"`
	CurrentPeriodEnd  *time.Time `json:"current_period_end,omitempty"`
	CancelAtPeriodEnd bool       `json:"cancel_at_period_end"`
	HasCustomer       bool       `json:"has_customer"`
	HasSubscription   bool       `json:"has_subscription"`
}

type BillingCheckoutRequest struct {
	Plan Plan `json:"plan"`
}

type BillingSessionResponse struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}
