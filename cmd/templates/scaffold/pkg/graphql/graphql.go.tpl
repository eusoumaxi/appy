package graphql

import (
	"{{.projectName}}/pkg/app"
	"{{.projectName}}/pkg/graphql/generated"
	"{{.projectName}}/pkg/graphql/resolver"

	gqlgen "github.com/99designs/gqlgen/graphql"
)

func init() {
	app.Server.SetupGraphQL(
		"/graphql",
		generated.NewExecutableSchema(newConfig()),
		[]gqlgen.HandlerExtension{},
	)
}

func newConfig() generated.Config {
	return generated.Config{
		Resolvers:  &resolver.Root{},
		Directives: generated.DirectiveRoot{},
	}
}
