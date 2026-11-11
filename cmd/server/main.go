// Command server is the Star Defender web back-end. It serves the game
// client (static files + WebAssembly) and a small JSON API for high scores
// and user-made levels.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	webDir := flag.String("web", "web", "directory with index.html and game.wasm")
	dataDir := flag.String("data", "data", "directory for high scores and custom levels")
	flag.Parse()

	if _, err := os.Stat(*webDir + "/game.wasm"); err != nil {
		log.Printf("warning: %s/game.wasm not found - build the client first (run `make`)", *webDir)
	}
	st, err := openStore(*dataDir)
	if err != nil {
		log.Fatalf("opening data dir: %v", err)
	}
	srv := &http.Server{
		Addr:              *addr,
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

	log.Printf("Star Defender listening on %s", *addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
