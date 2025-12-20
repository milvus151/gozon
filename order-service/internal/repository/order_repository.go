package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"order-service/internal/domain"
)

type OrderRepository struct {
	db *sql.DB
}

func NewOrderRepository(db *sql.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

func (r *OrderRepository) CreateOrderWithOutbox(order *domain.Order) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	queryOrder := `
	INSERT INTO orders (user_id, amount, status) 
	VALUES ($1, $2, $3)
	RETURNING id, created_at`
	err = tx.QueryRow(queryOrder, order.UserID, order.Amount, domain.OrderStatusPending).
		Scan(&order.ID, &order.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert order: %w", err)
	}
	payloadMap := map[string]interface{}{
		"order_id": order.ID,
		"user_id":  order.UserID,
		"amount":   order.Amount,
	}
	payloadBytes, _ := json.Marshal(payloadMap)
	queryOutbox := `
	INSERT INTO outbox (event_type, payload, status)
	VALUES ($1, $2, 'pending')`
	_, err = tx.Exec(queryOutbox, "OrderCreated", payloadBytes)
	if err != nil {
		return fmt.Errorf("failed to insert outbox: %w", err)
	}
	return tx.Commit()
}
