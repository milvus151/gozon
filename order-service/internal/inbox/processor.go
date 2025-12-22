package inbox

import (
	"database/sql"
	"encoding/json"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

type InboxProcessor struct {
	db       *sql.DB
	rabbitCh *amqp.Channel
}

func NewInboxProcessor(db *sql.DB, conn *amqp.Connection) (*InboxProcessor, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}
	_, err = ch.QueueDeclare(
		"payments_results_queue",
		true,
		false,
		false,
		false,
		nil)
	if err != nil {
		return nil, err
	}
	return &InboxProcessor{db: db, rabbitCh: ch}, nil
}

func (processor *InboxProcessor) Start() error {
	messages, err := processor.rabbitCh.Consume(
		"payments_results_queue",
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
		OrderID int64  `json:"order_id"`
		UserID  int64  `json:"user_id"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal(message.Body, &payload); err != nil {
		log.Printf("order inbox: json error: %v", err)
		message.Ack(false)
		return
	}
	log.Printf("order inbox: received payment result for order %d: %s", payload.OrderID, payload.Status)
	var newStatus string
	switch payload.Status {
	case "PaymentSucceeded":
		newStatus = "paid"
	case "PaymentFailed":
		newStatus = "payment_failed"
	default:
		log.Printf("order inbox: unknown status: %s", payload.Status)
		message.Ack(false)
		return
	}
	_, err := processor.db.Exec(`
        UPDATE orders
        SET status = $1
        WHERE id = $2
    `, newStatus, payload.OrderID)
	if err != nil {
		log.Printf("order inbox: update error: %v", err)
		message.Nack(false, true)
		return
	}
	message.Ack(false)
}
