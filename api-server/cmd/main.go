package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	// Chi allows you to route/handle any HTTP request method, such as all the usual suspects: GET, POST, HEAD, PUT, PATCH, DELETE, OPTIONS, TRACE, CONNECT
	// Routing refers to how an application's endpoints (URIs) respond to client requests.
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	// root
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Platform API Server"))
	})

	http.ListenAndServe(":8080", r)
}
