package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"payment-service/internal/handler"
	"payment-service/internal/inbox"
	"payment-service/internal/outbox"
	"payment-service/internal/repository"
	"time"

	_ "github.com/lib/pq"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	connStr := "host=postgres port=5432 user=user password=password dbname=gozon sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("invalid connection string: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("db not reachable: %v", err)
	}
	log.Println("Connected to DB")
	if err := createTables(db); err != nil {
		log.Fatalf("failed to create tables: %v", err)
	}
	repo := repository.NewAccountRepository(db)
	h := handler.NewAccountHandler(repo)

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
	///
	inboxProc, err := inbox.NewInboxProcessor(db, rabbitConn, repo)
	if err != nil {
		log.Fatalf("failed to create inbox processor: %v", err)
	}
	if err := inboxProc.Start(); err != nil {
		log.Fatalf("Inbox start failed: %v", err)
	}

	outboxProc, err := outbox.NewOutboxProcessor(db, rabbitConn)
	if err != nil {
		log.Fatalf("Payments outbox init failed: %v", err)
	}
	outboxProc.Start()

	mux := http.NewServeMux()
	mux.HandleFunc("/accounts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			h.CreateAccount(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/accounts/deposit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			h.Deposit(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/accounts/balance", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			h.GetBalance(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	addr := ":8080"
	log.Println("Payment Service started")
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func createTables(db *sql.DB) error {
	query := `
    CREATE TABLE IF NOT EXISTS accounts (
        id SERIAL PRIMARY KEY,
        user_id BIGINT NOT NULL UNIQUE,
        balance BIGINT NOT NULL DEFAULT 0,
        created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
    );
    CREATE TABLE IF NOT EXISTS account_transactions (
        id SERIAL PRIMARY KEY,
        account_id BIGINT NOT NULL REFERENCES accounts(id),
        amount BIGINT NOT NULL,
        created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
    );
    CREATE TABLE IF NOT EXISTS inbox_messages (
        id SERIAL PRIMARY KEY,
        message_id VARCHAR(255) NOT NULL UNIQUE,
        type VARCHAR(100) NOT NULL,
        payload JSONB NOT NULL,
        created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
    );
    CREATE TABLE IF NOT EXISTS outbox_events (
        id SERIAL PRIMARY KEY,
        type VARCHAR(100) NOT NULL,
        payload JSONB NOT NULL,
        status VARCHAR(20) NOT NULL DEFAULT 'pending',
        created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
    );`
	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("exec schema: %w", err)
	}
	log.Println("payments_db tables created/checked")
	return nil
}
