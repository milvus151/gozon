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
	RETURNING id, created_at, status`
	err = tx.QueryRow(queryOrder, order.UserID, order.Amount, domain.OrderStatusPending).
		Scan(&order.ID, &order.CreatedAt, &order.Status)
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

func (r *OrderRepository) GetOrderByID(orderID int64) (*domain.Order, error) {
	query := `SELECT id, user_id, amount, status, created_at FROM orders WHERE id = $1`
	o := &domain.Order{}
	err := r.db.QueryRow(query, orderID).Scan(
		&o.ID,
		&o.UserID,
		&o.Amount,
		&o.Status,
		&o.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("order not found")
		}
		return nil, fmt.Errorf("get order by id error: %w", err)
	}
	return o, nil
}

func (r *OrderRepository) GetOrdersByUserID(userID int64) ([]domain.Order, error) {
	query := `SELECT id, user_id, amount, status, created_at 
				FROM orders 
				WHERE user_id = $1
				ORDER BY created_at DESC`
	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("get orders by user_id error: %w", err)
	}
	defer rows.Close()
	var orders []domain.Order
	for rows.Next() {
		var order domain.Order
		if err := rows.Scan(&order.ID, &order.UserID, &order.Amount, &order.Status, &order.CreatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}
	return orders, nil
}
