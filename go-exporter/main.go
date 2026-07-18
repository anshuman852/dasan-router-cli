package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// ---- CLI flags ----
	host := flag.String("host", envDefault("DASAN_HOST", "192.168.1.1"), "Router IP/hostname")
	username := flag.String("username", envDefault("DASAN_USERNAME", ""), "Router login username")
	password := flag.String("password", envDefault("DASAN_PASSWORD", ""), "Router login password")
	port := flag.Int("port", 9800, "Exporter HTTP listen port")
	interval := flag.Int("interval", 60, "Scrape interval in seconds")
	flag.Parse()

	if *username == "" || *password == "" {
		log.Fatal("Username and password are required. Set DASAN_USERNAME/DASAN_PASSWORD or use -username/-password.")
	}

	// ---- Login ----
	log.Printf("Connecting to router at %s ...", *host)
	client := NewDasanClient(*host)
	if err := client.Login(*username, *password); err != nil {
		log.Fatalf("Login failed: %v", err)
	}
	log.Println("Logged in successfully")

	// ---- Initial scrape ----
	collector := NewCollector(client)
	collector.Collect()
	log.Println("Initial scrape completed")

	// ---- HTTP server ----
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		addr := fmt.Sprintf(":%d", *port)
		log.Printf("Prometheus metrics listening on %s", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Fatalf("HTTP server: %v", err)
		}
	}()

	// ---- Background collection loop ----
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(time.Duration(*interval) * time.Second)
	defer ticker.Stop()

	log.Printf("Background collection every %ds (slow objects every 300s)", *interval)
	for {
		select {
		case <-ticker.C:
			collector.Collect()
		case <-stop:
			log.Println("Shutting down ...")
			return
		}
	}
}

// envDefault returns the value of the environment variable key, or def if unset.
func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
