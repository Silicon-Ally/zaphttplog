package main

import (
	"log"
	"net/http"

	"github.com/Silicon-Ally/zaphttplog"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

func main() {
	// Configure your logger for your environment.
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("failed to init logger: %v", err)
	}

	// Initialize the router and set basic middleware.
	r := chi.NewRouter()
	r.Use(zaphttplog.NewMiddleware(logger))
	r.Use(middleware.Recoverer)

	// An example handler.
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("root."))
	})

	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatalf("http.ListenAndServe: %v", err)
	}
}
