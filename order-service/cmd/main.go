package main

import (
	"database/sql"
	"log"
	"net/http"

	"order-service/internal/handler"
	"order-service/internal/outbox"
	"order-service/internal/repository"

	_ "github.com/lib/pq"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	connStr := "host=localhost port=5435 user=user password=password dbname=orders_db sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Error connecting to DB: ", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatal("DB is not reachable: ", err)
	}
	log.Println("Connected to database successfully!")

	createTables(db)

	rabbitURL := "amqp://user:password@localhost:5672"
	rabbitConn, err := amqp.Dial(rabbitURL)
	if err != nil {
		log.Fatal("Error connecting to RabbitMQ: ", err)
	}
	defer rabbitConn.Close()
	processor, err := outbox.NewOutboxProcessor(db, rabbitConn)
	if err != nil {
		log.Fatal("Error init outbox processor: ", err)
	}
	orderRepo := repository.NewOrderRepository(db)
	orderHandler := handler.NewOrderHandler(orderRepo)

	processor.Start()

	mux := http.NewServeMux()
	mux.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			orderHandler.CreateOrder(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	serverPort := ":8080"
	log.Printf("Order Service starting on port %s...", serverPort)
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
        status VARCHAR(50) NOT NULL DEFAULT 'pending',
        created_at TIMESTAMP NOT NULL DEFAULT NOW()
    );
    CREATE TABLE IF NOT EXISTS outbox (
        id SERIAL PRIMARY KEY,
        event_type VARCHAR(50) NOT NULL,
        payload JSONB NOT NULL,
        status VARCHAR(20) NOT NULL DEFAULT 'pending',
        created_at TIMESTAMP NOT NULL DEFAULT NOW()
    );`
	_, err := db.Exec(query)
	if err != nil {
		log.Fatal("Failed to create tables:", err)
	}
	log.Println("Tables created!")
}
