package sdk

import (
	"os"
	"strings"

	"github.com/BryanMwangi/pine"
)

// registerStaticUI mounts a SPA static file server on the Pine app.
// All unmatched paths fall back to ui/dist/index.html so client-side routing works.
// Extension-specific routes registered by OnStart take priority because they are
// registered before this function is called.
func registerStaticUI(app *pine.Server) {
	app.Get("/*", func(c *pine.Ctx) error {
		p := strings.TrimPrefix(c.Params("*"), "/")
		if p == "" {
			p = "index.html"
		}
		full := "ui/dist/" + p

		if _, err := os.Stat(full); os.IsNotExist(err) {
			return c.SendFile("ui/dist/index.html")
		}
		return c.SendFile(full)
	})
}
