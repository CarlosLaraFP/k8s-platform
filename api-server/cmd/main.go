package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	h "api-server/internal/handler"
	m "api-server/internal/metrics"
)

func main() {
	// Chi allows you to route/handle any HTTP request method, such as all the usual suspects: GET, POST, HEAD, PUT, PATCH, DELETE, OPTIONS, TRACE, CONNECT
	// Routing refers to how an application's endpoints (URIs) respond to client requests.
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second)) // context deadline

	client := h.NewKubernetesClient()
	metrics := m.InitPrometheus()
	r.Handle("/metrics", promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{}))

	handler := &h.Handler{
		Claimer: client, // client is NewKubernetesClient()
		Metrics: metrics,
	}

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/templates/index.html")
	})
	// This tells Chi to match paths like /view/MyClaim, and now MakeHandler will receive the correct r.URL.Path value and extract MyClaim.
	r.Get("/view/{name}", h.MakeHandler(h.ViewHandler))
	r.Get("/edit/{name}", h.MakeHandler(h.EditHandler))
	r.Post("/submit", handler.SubmitHandler)
	r.Post("/submit/{name}", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/", http.StatusFound) })
	r.Get("/claims", handler.GetClaims)

	fmt.Println("Starting server...")
	log.Fatal(http.ListenAndServe(":8080", r))
}
