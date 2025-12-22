package inbox

import (
	"database/sql"
	"encoding/json"
	"log"
	"payment-service/internal/repository"

	amqp "github.com/rabbitmq/amqp091-go"
)

type InboxProcessor struct {
	db       *sql.DB
	rabbitCh *amqp.Channel
	repo     *repository.AccountRepository
}

func NewInboxProcessor(db *sql.DB, conn *amqp.Connection, repo *repository.AccountRepository) (*InboxProcessor, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}
	_, err = ch.QueueDeclare(
		"orders_queue",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return nil, err
	}
	return &InboxProcessor{db: db, rabbitCh: ch, repo: repo}, nil
}

func (processor *InboxProcessor) Start() error {
	messages, err := processor.rabbitCh.Consume(
		"orders_queue",
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}
	go func() {
		for message := range messages {
			processor.processMessage(message)
		}
	}()
	return nil
}

func (processor *InboxProcessor) processMessage(message amqp.Delivery) {
	var payload struct {
		OrderID int64 `json:"order_id"`
		UserID  int64 `json:"user_id"`
		Amount  int64 `json:"amount"`
	}
	if err := json.Unmarshal(message.Body, &payload); err != nil {
		log.Printf("Error decoding JSON: %v", err)
		message.Ack(false)
		return
	}
	log.Printf("Received payment request: OrderID=%d UserID=%d Amount=%d",
		payload.OrderID, payload.UserID, payload.Amount)
	tx, err := processor.db.Begin()
	if err != nil {
		log.Printf("DB error (begin tx): %v", err)
		message.Nack(false, true)
		return
	}
	defer tx.Rollback()
	success := false
	var accountID int64
	var balance int64
	err = tx.QueryRow(`
        SELECT id, balance FROM accounts
        WHERE user_id = $1 FOR UPDATE
    `, payload.UserID).Scan(&accountID, &balance)
	if err == sql.ErrNoRows {
		log.Printf("User %d not found, payment failed", payload.UserID)
		success = false
	} else if err != nil {
		log.Printf("DB error (select account): %v", err)
		message.Nack(false, true)
		return
	} else {
		if balance >= payload.Amount {
			_, err = tx.Exec(`
                UPDATE accounts SET balance = balance - $1 WHERE id = $2
            `, payload.Amount, accountID)
			if err != nil {
				log.Printf("DB error (update balance): %v", err)
				message.Nack(false, true)
				return
			}
			minusAmount := -payload.Amount
			_, err = tx.Exec(`
                INSERT INTO account_transactions (account_id, amount)
                VALUES ($1, $2)
            `, accountID, minusAmount)
			if err != nil {
				log.Printf("DB error (insert transaction): %v", err)
				message.Nack(false, true)
				return
			}
			success = true
			log.Printf("Payment SUCCESS for Order %d", payload.OrderID)
		} else {
			log.Printf("Not enough money for Order %d (Balance: %d, Needed: %d)",
				payload.OrderID, balance, payload.Amount)
			success = false
		}
	}
	resultEventType := "PaymentFailed"
	if success {
		resultEventType = "PaymentSucceeded"
	}
	resultPayload, err := json.Marshal(map[string]interface{}{
		"order_id": payload.OrderID,
		"user_id":  payload.UserID,
		"status":   resultEventType,
	})
	if err != nil {
		log.Printf("Failed to marshal result payload: %v", err)
		message.Nack(false, true)
		return
	}
	_, err = tx.Exec(`
        INSERT INTO outbox_events (type, payload, status)
        VALUES ($1, $2, 'pending')
    `, resultEventType, resultPayload)
	if err != nil {
		log.Printf("Failed to insert outbox: %v", err)
		message.Nack(false, true)
		return
	}
	if err := tx.Commit(); err != nil {
		log.Printf("Failed to commit tx: %v", err)
		message.Nack(false, true)
		return
	}
	message.Ack(false)
}
