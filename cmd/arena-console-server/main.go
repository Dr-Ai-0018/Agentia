package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ai-arena/internal/consoleapi"
)

func main() {
	addr := flag.String("addr", strings.TrimSpace(os.Getenv("ARENA_CONSOLE_ADDR")), "HTTP listen address (required; e.g. 127.0.0.1:8788)")
	root := flag.String("root", envOrDefault("ARENA_ROOT", ".agents"), "Arena state root")
	flag.Parse()

	if strings.TrimSpace(*addr) == "" {
		fmt.Fprintln(os.Stderr, "arena-console-server: --addr or ARENA_CONSOLE_ADDR is required")
		os.Exit(2)
	}
	server := &http.Server{
		Addr:              strings.TrimSpace(*addr),
		Handler:           consoleapi.New(consoleapi.Options{Root: *root, Token: os.Getenv("ARENA_CONSOLE_TOKEN")}).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errs := make(chan error, 1)
	fmt.Fprintf(os.Stderr, "arena-console-server listening on %s root=%s\n", server.Addr, *root)
	go func() {
		errs <- server.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(os.Stderr, "arena-console-server shutdown: %v\n", err)
			os.Exit(1)
		}
		if err := <-errs; err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "arena-console-server: %v\n", err)
			os.Exit(1)
		}
	case err := <-errs:
		if err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "arena-console-server: %v\n", err)
			os.Exit(1)
		}
	}
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
