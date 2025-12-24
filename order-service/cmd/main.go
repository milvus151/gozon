package main

import (
	"database/sql"
	"log"
	"net/http"
	"order-service/internal/inbox"
	"time"

	"order-service/internal/handler"
	"order-service/internal/outbox"
	"order-service/internal/repository"

	_ "github.com/lib/pq"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	connStr := "host=postgres port=5432 user=user password=password dbname=gozon sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Error connecting to DB: ", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatal("DB is not reachable: ", err)
	}
	log.Println("Connected to DB")

	createTables(db)

	rabbitURL := "amqp://user:password@rabbitmq:5672"
	var rabbitConn *amqp.Connection

	for i := 0; i < 15; i++ {
		rabbitConn, err = amqp.Dial(rabbitURL)
		if err == nil {
			log.Println("Connected to RabbitMQ")
			break
		}
		log.Printf("Failed to connect to RabbitMQ (attempt %d/15): %v. Retrying in 2s...", i+1, err)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		log.Fatalf("Could not connect to RabbitMQ after retries: %v", err)
	}
	defer rabbitConn.Close()
	processor, err := outbox.NewOutboxProcessor(db, rabbitConn)
	if err != nil {
		log.Fatal("Error init outbox processor: ", err)
	}
	orderRepo := repository.NewOrderRepository(db)
	orderHandler := handler.NewOrderHandler(orderRepo)

	processor.Start()
	orderInbox, err := inbox.NewInboxProcessor(db, rabbitConn)
	if err != nil {
		log.Fatalf("Order inbox init failed: %v", err)
	}
	if err := orderInbox.Start(); err != nil {
		log.Fatalf("Order inbox start failed: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			orderHandler.CreateOrder(w, r)
		} else if r.Method == http.MethodGet {
			orderHandler.GetOrdersByUserID(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/orders/by-id", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			orderHandler.GetOrderByID(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	serverPort := ":8080"
	log.Println("Order Service started")
	if err := http.ListenAndServe(serverPort, mux); err != nil {
		log.Fatal("Server failed:", err)
	}
}

func createTables(db *sql.DB) {
	query := `
    CREATE TABLE IF NOT EXISTS orders (
        id SERIAL PRIMARY KEY,
        user_id BIGINT NOT NULL,
        amount BIGINT NOT NULL,
        status VARCHAR(50) NOT NULL DEFAULT 'new',
        created_at TIMESTAMP NOT NULL DEFAULT NOW()
    );
    CREATE TABLE IF NOT EXISTS outbox (
        id SERIAL PRIMARY KEY,
        event_type VARCHAR(50) NOT NULL,
        payload JSONB NOT NULL,
        status VARCHAR(20) NOT NULL DEFAULT 'new',
        created_at TIMESTAMP NOT NULL DEFAULT NOW()
    );`
	_, err := db.Exec(query)
	if err != nil {
		log.Fatal("Failed to create tables:", err)
	}
	log.Println("Tables created!")
}
