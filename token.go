package sdk

import (
	"net/http"
	"os"
	"strings"

	"github.com/BryanMwangi/pine"
	"github.com/golang-jwt/jwt/v5"
)

// systemUserID is the sentinel value placed in extension tokens by the Tabibu
// server. Mirrors auth.SystemUserID in the server package.
const systemUserID = "00000000-0000-0000-0000-000000000000"

// tabibuClaims mirrors server's auth.Claims so the SDK can parse tokens issued
// by the Tabibu server without importing the server package.
type tabibuClaims struct {
	UserID       string   `json:"user_id"`
	Email        string   `json:"email"`
	RoleID       int      `json:"role_id"`
	DepartmentID int64    `json:"department_id"`
	IsSuperAdmin bool     `json:"is_super_admin"`
	Privileges   []string `json:"privileges,omitempty"`
	jwt.RegisteredClaims
}

func (c tabibuClaims) isExtension() bool { return c.UserID == systemUserID }

// ValidateWebViewToken is a Pine middleware that validates the ?token= query
// parameter on WebView routes. It ensures:
//   - the token was signed with TABIBU_JWT_SECRET
//   - the token was issued to an extension (UserID == systemUserID)
//   - the token's Subject matches this extension's name
//
// The token is the same JWT the Tabibu app fetches via
// GET /v1/admin/extensions/:name/token before launching the WebView.
func ValidateWebViewToken(extName string) pine.Middleware {
	secret := os.Getenv("TABIBU_JWT_SECRET")
	return func(next pine.Handler) pine.Handler {
		return func(c *pine.Ctx) error {
			tok := c.Query("token")
			if tok == "" {
				tok = strings.TrimPrefix(c.Header("Authorization"), "Bearer ")
			}
			if tok == "" {
				return c.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "token required"})
			}
			claims, err := parseTabibuToken(tok, secret)
			if err != nil || !claims.isExtension() {
				return c.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "invalid token"})
			}
			if claims.Subject != extName {
				return c.Status(http.StatusForbidden).JSON(map[string]string{"error": "token not issued for this extension"})
			}
			return next(c)
		}
	}
}

func parseTabibuToken(tokenStr, secret string) (*tabibuClaims, error) {
	var claims tabibuClaims
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
