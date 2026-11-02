package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"careerpilot/platform/internal/app"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	handler, startMonitoring, err := app.NewHandlerWithMonitor(filepath.Join(dataDirectory(), "careerpilot.db"))
	if err != nil {
		return err
	}

	host, port := "127.0.0.1", os.Getenv("API_PORT")
	if railwayPort := os.Getenv("PORT"); railwayPort != "" {
		host, port = "0.0.0.0", railwayPort
	}
	if port == "" {
		port = "8787"
	}
	if dir := os.Getenv("CONSOLE_DIR"); dir != "" {
		handler = withConsole(handler, dir)
	}

	server := &http.Server{
		Addr:              host + ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      110 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startMonitoring(ctx)

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("CareerPilot Go API listening on http://%s", server.Addr)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}

func dataDirectory() string {
	if dataDir := os.Getenv("CAREERPILOT_DATA_DIR"); dataDir != "" {
		return dataDir
	}
	workingDirectory, err := os.Getwd()
	if err == nil && filepath.Base(workingDirectory) == "platform" {
		return filepath.Join("..", ".data")
	}
	return ".data"
}

// withConsole serves the built web console, sending /api/ requests to the API and
// falling back to index.html for client-side routes.
func withConsole(api http.Handler, dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			api.ServeHTTP(w, r)
			return
		}
		if info, err := os.Stat(filepath.Join(dir, filepath.Clean("/"+r.URL.Path))); err != nil || info.IsDir() {
			if r.URL.Path != "/" {
				r.URL.Path = "/"
			}
		}
		files.ServeHTTP(w, r)
	})
}
