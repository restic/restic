package main

import (
	"log"
	"net/http"
	"strings"
)

type Route struct {
	path    []string
	handler http.Handler
}

type Router struct {
	routes map[string][]Route
}

func NewRouter() *Router {
	return &Router{make(map[string][]Route)}
}

func (router *Router) Options(path string, handler http.Handler) {
	router.Handle("OPTIONS", path, handler)
}

func (router *Router) OptionsFunc(path string, handler http.HandlerFunc) {
	router.Handle("OPTIONS", path, handler)
}

func (router *Router) Get(path string, handler http.Handler) {
	router.Handle("GET", path, handler)
}

func (router *Router) GetFunc(path string, handler http.HandlerFunc) {
	router.Handle("GET", path, handler)
}

func (router *Router) Head(path string, handler http.Handler) {
	router.Handle("HEAD", path, handler)
}

func (router *Router) HeadFunc(path string, handler http.HandlerFunc) {
	router.Handle("HEAD", path, handler)
}

func (router *Router) Post(path string, handler http.Handler) {
	router.Handle("POST", path, handler)
}

func (router *Router) PostFunc(path string, handler http.HandlerFunc) {
	router.Handle("POST", path, handler)
}

func (router *Router) Put(path string, handler http.Handler) {
	router.Handle("PUT", path, handler)
}

func (router *Router) PutFunc(path string, handler http.HandlerFunc) {
	router.Handle("PUT", path, handler)
}

func (router *Router) Delete(path string, handler http.Handler) {
	router.Handle("DELETE", path, handler)
}

func (router *Router) DeleteFunc(path string, handler http.HandlerFunc) {
	router.Handle("DELETE", path, handler)
}

func (router *Router) Trace(path string, handler http.Handler) {
	router.Handle("TRACE", path, handler)
}

func (router *Router) TraceFunc(path string, handler http.HandlerFunc) {
	router.Handle("TRACE", path, handler)
}

func (router *Router) Connect(path string, handler http.Handler) {
	router.Handle("Connect", path, handler)
}

func (router *Router) ConnectFunc(path string, handler http.HandlerFunc) {
	router.Handle("Connect", path, handler)
}

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
