package api

import (
	"net/http"
	"os"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gorilla/mux"
)

func HTTPHandler(resolver *Resolver, gqlEndpoint string) http.Handler {
	router := mux.NewRouter().SkipClean(true)
	router.Methods("POST").Handler(
		handler.NewDefaultServer(NewExecutableSchema(Config{
			Resolvers: resolver,
		})),
	)
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
