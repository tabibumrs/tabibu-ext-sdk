package sdk

import (
	"net/http"
	"os"
	"strings"

	"github.com/BryanMwangi/pine"
	"github.com/golang-jwt/jwt/v5"
)

// extensionClaims mirrors the server's auth.ExtensionClaims.
// The token is issued by the Tabibu server and validated here using the shared
// TABIBU_JWT_SECRET.
type extensionClaims struct {
	ExtensionName string   `json:"ext"`
	Privileges    []string `json:"privileges,omitempty"`
	jwt.RegisteredClaims
}

// ValidateWebViewToken is a Pine middleware that validates the ?token= query
// parameter on WebView routes. It ensures the token was issued for this
// specific extension.
//
// The token is the same JWT the Tabibu app fetches via
// GET /v1/admin/extensions/:name/token before launching the WebView.
func ValidateWebViewToken(extName string) pine.Middleware {
	secret := os.Getenv("TABIBU_JWT_SECRET")
	return func(next pine.Handler) pine.Handler {
		return func(c *pine.Ctx) error {
			tok := c.Query("token")
			if tok == "" {
				// Also accept Authorization: Bearer <token>
				hdr := c.Header("Authorization")
				tok = strings.TrimPrefix(hdr, "Bearer ")
			}
			if tok == "" {
				return c.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "token required"})
			}
			claims, err := parseExtensionToken(tok, secret)
			if err != nil {
				return c.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "invalid token"})
			}
			if claims.ExtensionName != extName {
				return c.Status(http.StatusForbidden).JSON(map[string]string{"error": "token not issued for this extension"})
			}
			return next(c)
		}
	}
}

func parseExtensionToken(tokenStr, secret string) (*extensionClaims, error) {
	var claims extensionClaims
	_, err := jwt.ParseWithClaims(tokenStr, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	return &claims, nil
}
