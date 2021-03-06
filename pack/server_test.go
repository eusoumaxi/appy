package pack

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/appist/appy/mailer"
	"github.com/appist/appy/support"
	"github.com/appist/appy/test"
	"github.com/gorilla/websocket"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

type serverSuite struct {
	test.Suite
	asset  *support.Asset
	config *support.Config
	i18n   *support.I18n
	logger *support.Logger
	mailer *mailer.Engine
	buffer *bytes.Buffer
	writer *bufio.Writer
}

func (s *serverSuite) SetupTest() {
	os.Setenv("APPY_ENV", "development")
	os.Setenv("APPY_MASTER_KEY", "58f364f29b568807ab9cffa22c99b538")
	os.Setenv("HTTP_CSRF_SECRET", "481e5d98a31585148b8b1dfb6a3c0465")
	os.Setenv("HTTP_SESSION_SECRETS", "481e5d98a31585148b8b1dfb6a3c0465")

	s.asset = support.NewAsset(nil, "testdata/context")
	s.logger, s.buffer, s.writer = support.NewTestLogger()
	s.config = support.NewConfig(s.asset, s.logger)
	s.i18n = support.NewI18n(s.asset, s.config, s.logger)
	s.mailer = mailer.NewEngine(s.asset, s.config, s.i18n, s.logger, nil)
}

func (s *serverSuite) TearDownTest() {
	os.Unsetenv("APPY_ENV")
	os.Unsetenv("APPY_MASTER_KEY")
	os.Unsetenv("HTTP_CSRF_SECRET")
	os.Unsetenv("HTTP_SESSION_SECRETS")
}

func (s *serverSuite) TestNewServerWithSSLEnabled() {
	s.config.HTTPSSLEnabled = true
	server := NewServer(s.asset, s.config, s.logger)

	s.NotNil(server.Config())
	s.NotNil(server.HTTP())
	s.NotNil(server.HTTPS())
	s.Equal(0, len(server.Middleware()))
	s.Equal("localhost:3000", server.HTTP().Addr)
	s.Equal("localhost:3443", server.HTTPS().Addr)
}

func (s *serverSuite) TestNewServerWithoutSSLEnabled() {
	server := NewServer(s.asset, s.config, s.logger)

	s.NotNil(server.Config())
	s.NotNil(server.HTTP())
	s.NotNil(server.HTTPS())
	s.Equal(0, len(server.Middleware()))
	s.Equal("localhost:3000", server.HTTP().Addr)
	s.Equal("localhost:3443", server.HTTPS().Addr)
}

func (s *serverSuite) TestNewAppServer() {
	server := NewAppServer(s.asset, s.config, s.i18n, s.mailer, s.logger, nil)

	s.Equal(15, len(server.middleware))
}

func (s *serverSuite) TestIsSSLCertsExisted() {
	server := NewServer(s.asset, s.config, s.logger)
	s.Equal(false, server.IsSSLCertExisted())

	s.config.HTTPSSLCertPath = "testdata/server/is_ssl_certs_existed"
	server = NewServer(s.asset, s.config, s.logger)
	s.Equal(true, server.IsSSLCertExisted())
}

func (s *serverSuite) TestInfo() {
	server := NewServer(s.asset, s.config, s.logger)
	output := server.Info()
	s.Contains(output, fmt.Sprintf("* appy 0.1.0 (%s), build: debug, environment: development, config: configs/.env.development", runtime.Version()))
	s.Contains(output, "* Listening on http://localhost:3000")

	s.config.HTTPSSLEnabled = true
	server = NewServer(s.asset, s.config, s.logger)
	output = server.Info()
	s.Contains(output, fmt.Sprintf("* appy 0.1.0 (%s), build: debug, environment: development, config: configs/.env.development", runtime.Version()))
	s.Contains(output, "* Listening on http://localhost:3000, https://localhost:3443")

	s.config.HTTPHost = "0.0.0.0"
	server = NewServer(s.asset, s.config, s.logger)
	output = server.Info()
	s.Contains(output, fmt.Sprintf("* appy 0.1.0 (%s), build: debug, environment: development, config: configs/.env.development", runtime.Version()))
	s.Contains(output, "* Listening on http://0.0.0.0:3000, https://0.0.0.0:3443")
}

func (s *serverSuite) TestRouting() {
	server := NewServer(s.asset, s.config, s.logger)
	server.ServeNoRoute()
	s.Equal(server.BasePath(), "/")

	w := server.TestHTTPRequest("GET", "/foobar", nil, nil)
	defer w.Close()

	s.Equal(http.StatusNotFound, w.Code)
	s.Contains(w.Body.String(), "<title>404 Page Not Found</title>")

	server.Any("/foo", func(c *Context) { c.String(http.StatusOK, "bar") })

	for _, method := range anyMethods {
		w := server.TestHTTPRequest(method, "/foo", nil, nil)
		defer w.Close()

		s.Equal(http.StatusOK, w.Code)
		s.Equal("bar", w.Body.String())
	}

	server.Handle("CONNECT", "/bar", func(c *Context) { c.String(http.StatusOK, "foo") })
	server.Handle("TRACE", "/bar", func(c *Context) { c.String(http.StatusOK, "foo") })
	server.DELETE("/bar", func(c *Context) { c.String(http.StatusOK, "foo") })
	server.GET("/bar", func(c *Context) { c.String(http.StatusOK, "foo") })
	server.HEAD("/bar", func(c *Context) { c.String(http.StatusOK, "foo") })
	server.OPTIONS("/bar", func(c *Context) { c.String(http.StatusOK, "foo") })
	server.PATCH("/bar", func(c *Context) { c.String(http.StatusOK, "foo") })
	server.PUT("/bar", func(c *Context) { c.String(http.StatusOK, "foo") })
	server.POST("/bar", func(c *Context) { c.String(http.StatusOK, "foo") })

	for _, method := range anyMethods {
		w := server.TestHTTPRequest(method, "/bar", nil, nil)
		defer w.Close()

		s.Equal(http.StatusOK, w.Code)
		s.Equal("foo", w.Body.String())
	}

	count := 1
	v1 := server.Group("/v1")
	v1.Use(func(c *Context) {
		count = 10
		c.Next()
	})
	v1.Any("/foo", func(c *Context) { c.String(http.StatusOK, "bar") })

	adminV1 := v1.Group("/admin")
	adminV1.GET("/orders", func(c *Context) { c.String(http.StatusOK, "admin") })

	for _, method := range anyMethods {
		w := server.TestHTTPRequest(method, "/v1/foo", nil, nil)
		defer w.Close()

		s.Equal(http.StatusOK, w.Code)
		s.Equal("bar", w.Body.String())
	}
	s.Equal(10, count)

	v1.Handle("CONNECT", "/bar", func(c *Context) { c.String(http.StatusOK, "foo") })
	v1.Handle("TRACE", "/bar", func(c *Context) { c.String(http.StatusOK, "foo") })
	v1.DELETE("/bar", func(c *Context) { c.String(http.StatusOK, "foo") })
	v1.GET("/bar", func(c *Context) { c.String(http.StatusOK, "foo") })
	v1.HEAD("/bar", func(c *Context) { c.String(http.StatusOK, "foo") })
	v1.OPTIONS("/bar", func(c *Context) { c.String(http.StatusOK, "foo") })
	v1.PATCH("/bar", func(c *Context) { c.String(http.StatusOK, "foo") })
	v1.PUT("/bar", func(c *Context) { c.String(http.StatusOK, "foo") })
	v1.POST("/bar", func(c *Context) { c.String(http.StatusOK, "foo") })

	for _, method := range anyMethods {
		w := server.TestHTTPRequest(method, "/v1/bar", nil, nil)
		defer w.Close()

		s.Equal(http.StatusOK, w.Code)
		s.Equal("foo", w.Body.String())
	}

	routes := server.Routes()
	s.Equal(37, len(routes))

	route := routes[len(routes)-12]
	recorder := httptest.NewRecorder()
	c, _ := NewTestContext(recorder)
	s.Equal("/v1/foo", route.Path)

	route.HandlerFunc(c)
	s.Equal(http.StatusOK, recorder.Code)
	s.Equal("bar", recorder.Body.String())
}

func (s *serverSuite) TestServeSPA() {
	server := NewServer(s.asset, s.config, s.logger)
	server.ServeSPA("/", nil)
	w := server.TestHTTPRequest("GET", "/", nil, nil)
	defer w.Close()

	// Since reverse proxy is working in test and the webpack-dev-server not running, it should throw 502.
	s.Equal(http.StatusBadGateway, w.Code)

	s.config.HTTPSSLEnabled = true
	server = NewServer(s.asset, s.config, s.logger)
	server.ServeSPA("/", nil)
	w = server.TestHTTPRequest("GET", "/", nil, nil)

	// Since reverse proxy is working in test and the webpack-dev-server not running, it should throw 502.
	s.Equal(http.StatusBadGateway, w.Code)
}

type fakeGQLExt struct{}

var _ interface {
	graphql.OperationParameterMutator
	graphql.HandlerExtension
} = fakeGQLExt{}

func (c fakeGQLExt) ExtensionName() string {
	return "fakeGQLExt"
}

func (c fakeGQLExt) Validate(schema graphql.ExecutableSchema) error {
	return nil
}

func (c fakeGQLExt) MutateOperationParameters(ctx context.Context, rawParams *graphql.RawParams) *gqlerror.Error {
	return nil
}

func (s *serverSuite) TestSetupGraphQL() {
	os.Setenv("APPY_ENV", "development")
	os.Setenv("APPY_MASTER_KEY", "58f364f29b568807ab9cffa22c99b538")
	os.Setenv("HTTP_CSRF_SECRET", "481e5d98a31585148b8b1dfb6a3c0465")
	os.Setenv("HTTP_SESSION_SECRETS", "481e5d98a31585148b8b1dfb6a3c0465")
	defer func() {
		os.Unsetenv("APPY_ENV")
		os.Unsetenv("APPY_MASTER_KEY")
		os.Unsetenv("HTTP_CSRF_SECRET")
		os.Unsetenv("HTTP_SESSION_SECRETS")
	}()

	s.config.GQLPlaygroundEnabled = true
	s.config.GQLPlaygroundPath = "/graphiql"
	graphqlPath := "/graphql"
	server := NewServer(s.asset, s.config, s.logger)
	server.Use(mdwCSRF(s.config, s.logger))
	server.SetupGraphQL(graphqlPath, nil, []graphql.HandlerExtension{fakeGQLExt{}})

	w := server.TestHTTPRequest("GET", "/graphiql", nil, nil)
	defer w.Close()

	s.Equal(200, w.Code)
	s.Contains(w.Body.String(), "<title>GraphQL Playground</title>")

	w = server.TestHTTPRequest("POST", graphqlPath, nil, nil)
	s.Equal(403, w.Code)

	w = server.TestHTTPRequest("POST", graphqlPath, H{
		"content-type": "application/json",
		"x-api-only":   "1",
	}, nil)
	s.Equal(422, w.Code)

	ts := httptest.NewServer(server.Router())
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http")+graphqlPath, nil)
	defer ws.Close()
	s.Nil(err)

	ws, _, err = websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http"), nil)
	s.NotNil(err)
}

func TestServerSuite(t *testing.T) {
	test.Run(t, new(serverSuite))
}
