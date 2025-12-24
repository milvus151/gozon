package outbox

import (
	"database/sql"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type OutboxProcessor struct {
	db         *sql.DB
	rabbitConn *amqp.Connection
	rabbitCh   *amqp.Channel
}

func NewOutboxProcessor(db *sql.DB, rabbitConn *amqp.Connection) (*OutboxProcessor, error) {
	ch, err := rabbitConn.Channel()
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
	return &OutboxProcessor{
		db:         db,
		rabbitConn: rabbitConn,
		rabbitCh:   ch,
	}, nil
}

func (p *OutboxProcessor) Start() {
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		for range ticker.C {
			p.processEvents()
		}
	}()
}

func (p *OutboxProcessor) processEvents() {
	rows, err := p.db.Query(`
	SELECT id, event_type, payload
	FROM outbox 
	WHERE status = 'new'
	LIMIT 10
	`)
	if err != nil {
		log.Println("Error reading outbox rows:", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var eventType string
		var payload []byte
		if err := rows.Scan(&id, &eventType, &payload); err != nil {
			continue
		}
		err := p.rabbitCh.Publish(
			"",
			"orders_queue",
			false,
			false,
			amqp.Publishing{
				ContentType: "application/json",
				Body:        payload,
				Type:        eventType,
			},
		)
		if err != nil {
			log.Printf("Failed to publish event %d: %v", id, err)
			continue
		}

		_, err = p.db.Exec("UPDATE outbox SET status = 'processed' WHERE id = $1", id)
		if err != nil {
			log.Printf("Failed to update status for event %d: %v", id, err)
		} else {
			log.Printf("Event %d sent to RabbitMQ!", id)
		}
	}
}
