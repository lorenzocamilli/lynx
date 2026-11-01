package api

import (
	"net/http"
	"os"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gorilla/mux"
	"github.com/vektah/gqlparser/v2/ast"
)

func HTTPHandler(resolver *Resolver, gqlEndpoint string) http.Handler {
	server := handler.New(NewExecutableSchema(Config{Resolvers: resolver}))
	// Keep the POST transports, query cache, introspection and persisted-query
	// behavior previously supplied by gqlgen's deprecated default server.
	// GET is handled separately below; Lynx has no GraphQL subscriptions.
	server.AddTransport(transport.POST{})
	server.AddTransport(transport.MultipartForm{})
	server.SetQueryCache(lru.New[*ast.QueryDocument](1000))
	server.Use(extension.Introspection{})
	server.Use(extension.AutomaticPersistedQuery{Cache: lru.New[string](100)})

	router := mux.NewRouter().SkipClean(true)
	router.Methods("POST").Handler(server)
	// Playground is only available when LYNX_DEV=1 to prevent information exposure in production.
	if os.Getenv("LYNX_DEV") == "1" {
		router.Methods("GET").Handler(playground.Handler("GraphQL Playground", gqlEndpoint))
	} else {
		router.Methods("GET").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		})
	}

	return router
}
