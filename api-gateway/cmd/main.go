package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	amqp "github.com/rabbitmq/amqp091-go"
)

type WSHub struct {
	clients map[int][]*websocket.Conn
	mu      sync.RWMutex
}

var hub = WSHub{
	clients: make(map[int][]*websocket.Conn),
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (h *WSHub) AddClient(userID int, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[userID] = append(h.clients[userID], conn)
	log.Printf("WebSocket connected: UserID=%d", userID)
}

func (h *WSHub) RemoveClient(userID int, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	conns := h.clients[userID]
	for i, c := range conns {
		if c == conn {
			h.clients[userID] = append(conns[:i], conns[i+1:]...)
			break
		}
	}
	if len(h.clients[userID]) == 0 {
		delete(h.clients, userID)
	}
	conn.Close()
	log.Printf("WebSocket disconnected: UserID=%d", userID)
}

func (h *WSHub) Broadcast(userID int, message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conns, ok := h.clients[userID]
	if !ok {
		return
	}
	for _, conn := range conns {
		err := conn.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			log.Printf("WS write error: %v", err)
			conn.Close()
		}
	}
}

func startRabbitMQListener() {
	rabbitURL := "amqp://user:password@rabbitmq:5672/"
	var conn *amqp.Connection
	var err error

	for i := 0; i < 15; i++ {
		conn, err = amqp.Dial(rabbitURL)
		if err == nil {
			log.Println("Gateway connected to RabbitMQ")
			break
		}
		log.Printf("  Retry RabbitMQ connection %d/15...", i+1)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("Gateway failed to connect to RabbitMQ: %v", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		log.Fatal(err)
	}

	err = ch.ExchangeDeclare("payment_events_fanout", "fanout", true, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}

	q, err := ch.QueueDeclare(
		"",
		false,
		true,
		true,
		false,
		nil,
	)
	if err != nil {
		log.Fatal(err)
	}

	err = ch.QueueBind(
		q.Name,
		"",
		"payment_events_fanout",
		false,
		nil,
	)
	if err != nil {
		log.Fatal(err)
	}

	msgs, err := ch.Consume(q.Name, "", true, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Gateway listening to payment_events_fanout")

	go func() {
		for d := range msgs {
			log.Printf("Gateway received event: %s", d.Body)

			var event struct {
				OrderID int    `json:"order_id"`
				Status  string `json:"status"`
				UserID  int    `json:"user_id"`
			}
			if err := json.Unmarshal(d.Body, &event); err != nil {
				continue
			}
			if event.UserID != 0 {
				hub.Broadcast(event.UserID, d.Body)
			}
		}
	}()
}

func proxyRequest(serviceURL string, w http.ResponseWriter, r *http.Request) {
	targetURL := serviceURL + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header = r.Header

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	userIDStr := r.URL.Query().Get("user_id")
	var userID int
	fmt.Sscanf(userIDStr, "%d", &userID)

	if userID == 0 {
		http.Error(w, "user_id required", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WS Upgrade error: %v", err)
		return
	}

	hub.AddClient(userID, conn)

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			hub.RemoveClient(userID, conn)
			break
		}
	}
}

func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	go startRabbitMQListener()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsHandler)

	mux.HandleFunc("/accounts", func(w http.ResponseWriter, r *http.Request) {
		proxyRequest("http://payment-service:8080", w, r)
	})
	mux.HandleFunc("/accounts/", func(w http.ResponseWriter, r *http.Request) {
		proxyRequest("http://payment-service:8080", w, r)
	})

	mux.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
		proxyRequest("http://order-service:8080", w, r)
	})
	mux.HandleFunc("/orders/", func(w http.ResponseWriter, r *http.Request) {
		proxyRequest("http://order-service:8080", w, r)
	})

	log.Println("Gateway starting on :8080")
	handler := enableCORS(mux)
	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatalf("Gateway failed: %v", err)
	}
}
