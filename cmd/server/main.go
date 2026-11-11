// Command server is the Star Defender web back-end. It serves the game
// client (static files + WebAssembly) and a small JSON API for high scores
// and user-made levels.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	port := flag.Int("port", 0, "port to listen on (default 8080, or $PORT if set)")
	addrFlag := flag.String("addr", "", "full listen address such as 127.0.0.1:9000 (overrides -port)")
	webDir := flag.String("web", "web", "directory with index.html and game.wasm")
	dataDir := flag.String("data", "data", "directory for high scores and custom levels")
	flag.Parse()

	addr, err := listenAddr(*addrFlag, *port, os.Getenv("PORT"))
	if err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stat(*webDir + "/game.wasm"); err != nil {
		log.Printf("warning: %s/game.wasm not found - build the client first (run `make`)", *webDir)
	}
	st, err := openStore(*dataDir)
	if err != nil {
		log.Fatalf("opening data dir: %v", err)
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           newHandler(st, *webDir),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	log.Printf("Star Defender listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// listenAddr picks the listen address: -addr wins, then -port, then the
// PORT environment variable, then 8080.
func listenAddr(addr string, port int, envPort string) (string, error) {
	if addr != "" {
		return addr, nil
	}
	if port == 0 && envPort != "" {
		p, err := strconv.Atoi(envPort)
		if err != nil {
			return "", fmt.Errorf("invalid PORT %q", envPort)
		}
		port = p
	}
	if port == 0 {
		port = 8080
	}
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("port %d is out of range (1-65535)", port)
	}
	return ":" + strconv.Itoa(port), nil
}
