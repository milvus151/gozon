package outbox

import (
	"database/sql"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type OutboxProcessor struct {
	db       *sql.DB
	rabbitCh *amqp.Channel
}

func NewOutboxProcessor(db *sql.DB, conn *amqp.Connection) (*OutboxProcessor, error) {
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
		nil,
	)
	if err != nil {
		return nil, err
	}
	return &OutboxProcessor{db: db, rabbitCh: ch}, nil
}

func (processor *OutboxProcessor) Start() {
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		for range ticker.C {
			processor.processEvents()
		}
	}()
}

func (processor *OutboxProcessor) processEvents() {
	rows, err := processor.db.Query(`
	SELECT id, type, payload FROM outbox_events
	WHERE status = 'pending'
	LIMIT 10
	`)
	if err != nil {
		log.Printf("payments outbox: query error: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var eventType string
		var payload []byte
		if err := rows.Scan(&id, &eventType, &payload); err != nil {
			log.Printf("payments outbox: row scan error: %v", err)
			continue
		}
		err = processor.rabbitCh.Publish(
			"",
			"payments_results_queue",
			false,
			false,
			amqp.Publishing{
				ContentType: "application/json",
				Body:        payload,
				Type:        eventType,
			},
		)
		if err != nil {
			log.Printf("payments outbox: publish error for id = %d: %v", id, err)
			continue
		}
		_, err = processor.db.Exec(`
			UPDATE outbox_events
			SET status = 'processed'
			WHERE id = $1
			`, id)
		if err != nil {
			log.Printf("payments outbox: update status error for id=%d: %v", id, err)
		} else {
			log.Printf("payments outbox: event %d (%s) sent", id, eventType)
		}
	}
}
