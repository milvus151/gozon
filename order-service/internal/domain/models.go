package domain

import "time"

type OrderStatus string

const (
	OrderStatusNew       OrderStatus = "new"
	OrderStatusFinished  OrderStatus = "finished"
	OrderStatusCancelled OrderStatus = "cancelled"
)

type Order struct {
	ID        int64       `json:"id"`
	UserID    int64       `json:"user_id"`
	Amount    int64       `json:"amount"`
	Status    OrderStatus `json:"status"`
	CreatedAt time.Time   `json:"created_at"`
}
