package main

import (
	"io"
	"log"
	"net/http"
	"net/url"
)

func main() {
	orderBase, _ := url.Parse("http://order-service:8080")
	paymentBase, _ := url.Parse("http://payment-service:8080")

	mux := http.NewServeMux()
	mux.HandleFunc("/orders", proxy(orderBase))
	mux.HandleFunc("/orders/by-id", proxy(orderBase))

	mux.HandleFunc("/accounts", proxy(paymentBase))
	mux.HandleFunc("/accounts/deposit", proxy(paymentBase))
	mux.HandleFunc("/accounts/balance", proxy(paymentBase))

	addr := ":8080"
	log.Printf("API Gateway starting on %s...", addr)
	handler := enableCORS(mux)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal("gateway failed:", err)
	}
}

func proxy(targetBase *url.URL) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target := *targetBase
		target.Path = r.URL.Path
		target.RawQuery = r.URL.RawQuery
		req, err := http.NewRequest(r.Method, target.String(), r.Body)
		if err != nil {
			http.Error(w, "failed to create request", http.StatusInternalServerError)
			return
		}
		req.Header = r.Header.Clone()
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, "upstream error: "+err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
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
