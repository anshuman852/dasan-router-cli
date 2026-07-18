// Package exporter implements the Prometheus metrics HTTP server.
package exporter

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/anshuman852/dasan/internal/client"
	"github.com/anshuman852/dasan/internal/collector"
)

// Serve starts the Prometheus exporter HTTP server.
// host is the router IP, interval is the scrape interval in seconds.
func Serve(cl *client.DasanClient, port, interval int) {
	// ---- Initial scrape ----
	coll := collector.NewCollector(cl)
	coll.Collect()
	log.Println("Initial scrape completed")

	// ---- HTTP server ----
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	go func() {
		addr := fmt.Sprintf(":%d", port)
		log.Printf("Prometheus metrics listening on %s", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Fatalf("HTTP server: %v", err)
		}
	}()

	// ---- Background collection loop ----
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	log.Printf("Background collection every %ds (slow objects every 300s)", interval)
	for {
		select {
		case <-ticker.C:
			coll.Collect()
		case <-stop:
			log.Println("Shutting down ...")
			cl.Logout()
			return
		}
	}
}
