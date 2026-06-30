package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"ai-arena/internal/consoleapi"
)

func main() {
	addr := flag.String("addr", envOrDefault("ARENA_CONSOLE_ADDR", "127.0.0.1:8787"), "HTTP listen address")
	root := flag.String("root", envOrDefault("ARENA_ROOT", ".agents"), "Arena state root")
	flag.Parse()

	server := &http.Server{
		Addr:              strings.TrimSpace(*addr),
		Handler:           consoleapi.New(consoleapi.Options{Root: *root}).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	fmt.Fprintf(os.Stderr, "arena-console-server listening on %s root=%s\n", server.Addr, *root)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "arena-console-server: %v\n", err)
		os.Exit(1)
	}
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
