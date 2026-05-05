package api

import (
	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/api/organizations"
	"github.com/livereview/internal/api/users"
)

// DocsBuilder provides a fake minimal environment to parse routes via d1vbyz3r0/typed.
type DocsBuilder struct {
	server *Server
}

func NewDocsBuilder() *DocsBuilder {
	s := &Server{
		echo:             echo.New(),
		deploymentConfig: &DeploymentConfig{Mode: "production"},
		authHandlers:     &auth.AuthHandlers{},
		tokenService:     &auth.TokenService{},
		userHandlers:     &users.UserHandlers{},
		profileHandlers:  &users.ProfileHandlers{},
		orgHandlers:      &organizations.OrganizationHandlers{},
		testHandlers:     &TestHandlers{},
		// More handlers will be mocked cleanly down in setupRoutes using s.db checks
		// but since they are initialized locally in setupRoutes, they are fine.
	}
	return &DocsBuilder{server: s}
}

func (b *DocsBuilder) Build() *Server {
	b.server.setupRoutes()
	return b.server
}

func (b *DocsBuilder) OnRouteAdded(onRouteAdded func(
	host string,
	route echo.Route,
	handler echo.HandlerFunc,
	middleware []echo.MiddlewareFunc,
)) {
	b.server.echo.OnAddRouteHandler = onRouteAdded
}

func (b *DocsBuilder) ProvideRoutes() {
	b.server.setupRoutes()
}
