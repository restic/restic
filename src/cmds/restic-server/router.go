// +build go1.4

package main

import (
	"log"
	"net/http"
	"strings"
)

// Route is a handler for a path that was already split.
type Route struct {
	path    []string
	handler http.Handler
}

// Router maps HTTP methods to a slice of Route handlers.
type Router struct {
	routes map[string][]Route
}

// NewRouter creates a new Router and returns a pointer to it.
func NewRouter() *Router {
	return &Router{make(map[string][]Route)}
}

// Options registers handler for path with method "OPTIONS".
func (router *Router) Options(path string, handler http.Handler) {
	router.Handle("OPTIONS", path, handler)
}

// OptionsFunc registers handler for path with method "OPTIONS".
func (router *Router) OptionsFunc(path string, handler http.HandlerFunc) {
	router.Handle("OPTIONS", path, handler)
}

// Get registers handler for path with method "GET".
func (router *Router) Get(path string, handler http.Handler) {
	router.Handle("GET", path, handler)
}

// GetFunc registers handler for path with method "GET".
func (router *Router) GetFunc(path string, handler http.HandlerFunc) {
	router.Handle("GET", path, handler)
}

// Head registers handler for path with method "HEAD".
func (router *Router) Head(path string, handler http.Handler) {
	router.Handle("HEAD", path, handler)
}

// HeadFunc registers handler for path with method "HEAD".
func (router *Router) HeadFunc(path string, handler http.HandlerFunc) {
	router.Handle("HEAD", path, handler)
}

// Post registers handler for path with method "POST".
func (router *Router) Post(path string, handler http.Handler) {
	router.Handle("POST", path, handler)
}

// PostFunc registers handler for path with method "POST".
func (router *Router) PostFunc(path string, handler http.HandlerFunc) {
	router.Handle("POST", path, handler)
}

// Put registers handler for path with method "PUT".
func (router *Router) Put(path string, handler http.Handler) {
	router.Handle("PUT", path, handler)
}

// PutFunc registers handler for path with method "PUT".
func (router *Router) PutFunc(path string, handler http.HandlerFunc) {
	router.Handle("PUT", path, handler)
}

// Delete registers handler for path with method "DELETE".
func (router *Router) Delete(path string, handler http.Handler) {
	router.Handle("DELETE", path, handler)
}

// DeleteFunc registers handler for path with method "DELETE".
func (router *Router) DeleteFunc(path string, handler http.HandlerFunc) {
	router.Handle("DELETE", path, handler)
}

// Trace registers handler for path with method "TRACE".
func (router *Router) Trace(path string, handler http.Handler) {
	router.Handle("TRACE", path, handler)
}

// TraceFunc registers handler for path with method "TRACE".
func (router *Router) TraceFunc(path string, handler http.HandlerFunc) {
	router.Handle("TRACE", path, handler)
}

// Connect registers handler for path with method "Connect".
func (router *Router) Connect(path string, handler http.Handler) {
	router.Handle("Connect", path, handler)
}

// ConnectFunc registers handler for path with method "Connect".
func (router *Router) ConnectFunc(path string, handler http.HandlerFunc) {
	router.Handle("Connect", path, handler)
}

// Handle registers a http.Handler for method and uri
func (router *Router) Handle(method string, uri string, handler http.Handler) {
	routes := router.routes[method]
	path := strings.Split(uri, "/")
	routes = append(routes, Route{path, handler})
	router.routes[method] = routes
}

func (router *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	method := r.Method
	uri := r.RequestURI
	path := strings.Split(uri, "/")

	log.Printf("%s %s", method, uri)

ROUTE:
	for _, route := range router.routes[method] {
		if len(route.path) != len(path) {
			continue
		}
		for i := 0; i < len(route.path); i++ {
			if !strings.HasPrefix(route.path[i], ":") && route.path[i] != path[i] {
				continue ROUTE
			}
		}
		route.handler.ServeHTTP(w, r)
		return
	}

	http.Error(w, "404 not found", 404)
}
