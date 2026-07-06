package sdk

import (
	"context"

	"github.com/BryanMwangi/pine"
)

// pineServer wraps *pine.Server and implements the Server interface so
// extension OnStart handlers don't need to import Pine directly.
type pineServer struct {
	app *pine.Server
}

var _ Server = (*pineServer)(nil)

func (s *pineServer) Get(path string, h HandlerFunc) {
	s.app.Get(path, adaptHandler(h))
}

func (s *pineServer) Post(path string, h HandlerFunc) {
	s.app.Post(path, adaptHandler(h))
}

func (s *pineServer) Put(path string, h HandlerFunc) {
	s.app.Put(path, adaptHandler(h))
}

func (s *pineServer) Delete(path string, h HandlerFunc) {
	s.app.Delete(path, adaptHandler(h))
}

// adaptHandler converts an SDK HandlerFunc into a pine.Handler.
func adaptHandler(h HandlerFunc) pine.Handler {
	return func(c *pine.Ctx) error {
		return h(&pineCtx{inner: c})
	}
}

// pineCtx wraps *pine.Ctx and implements the Ctx interface.
type pineCtx struct {
	inner *pine.Ctx
}

var _ Ctx = (*pineCtx)(nil)

func (c *pineCtx) Status(code int) Ctx {
	c.inner.Status(code)
	return c
}

func (c *pineCtx) JSON(v any) error {
	return c.inner.JSON(v)
}

func (c *pineCtx) BindJSON(v any) error {
	return c.inner.BindJSON(v)
}

func (c *pineCtx) Params(key string) string {
	return c.inner.Params(key)
}

func (c *pineCtx) Query(key string) string {
	return c.inner.Query(key)
}

func (c *pineCtx) Context() context.Context {
	return c.inner.Context()
}

func (c *pineCtx) Header(key string) string {
	return c.inner.Header(key)
}
