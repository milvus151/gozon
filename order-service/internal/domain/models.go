package domain

import "time"

type OrderStatus string

const (
	OrderStatusPending       OrderStatus = "pending"
	OrderStatusPaid          OrderStatus = "paid"
	OrderStatusPaymentFailed OrderStatus = "payment_failed"
)

type Order struct {
	ID        int64       `json:"id"`
	UserID    int64       `json:"user_id"`
	Amount    int64       `json:"amount"`
	Status    OrderStatus `json:"status"`
	CreatedAt time.Time   `json:"created_at"`
}
