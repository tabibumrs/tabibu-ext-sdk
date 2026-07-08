package sdk

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// TokenClaims holds the verified claims from a Tabibu extension WebView JWT.
// These tokens are issued by GET /v1/extensions/:name/token and signed with
// the ephemeral EXT_JWT_SECRET — a secret that is separate from Tabibu's
// main auth secret and rotates on every server restart.
type TokenClaims struct {
	// ExtensionName is the JWT subject — the extension this token was issued for.
	ExtensionName string
	// Privileges are the privilege strings the token holder holds.
	// These match the extension's required_privileges from manifest.toml.
	Privileges []string
}

// ValidateToken parses and validates a Tabibu extension WebView JWT.
//
// Use this to authenticate direct HTTP calls to your extension's own server
// that are NOT proxied through /v1/ui/:name/* (proxied calls are pre-validated
// by Tabibu). Call only after sdk.Run() has started — EXT_JWT_SECRET must be
// set in the process environment.
//
//	func (e *MyExt) requireAuth(c sdk.Ctx) (*sdk.TokenClaims, error) {
//	    tok := strings.TrimPrefix(c.Header("Authorization"), "Bearer ")
//	    if tok == "" {
//	        return nil, c.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "unauthorized"})
//	    }
//	    claims, err := sdk.ValidateToken(tok)
//	    if err != nil {
//	        return nil, c.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "invalid token"})
//	    }
//	    return claims, nil
//	}
func ValidateToken(tokenStr string) (*TokenClaims, error) {
	if _jwtSecret == "" {
		return nil, fmt.Errorf("sdk: EXT_JWT_SECRET not set — ValidateToken requires sdk.Run() to have started")
	}

	type rawClaims struct {
		Privileges []string `json:"privileges"`
		jwt.RegisteredClaims
	}

	tok, err := jwt.ParseWithClaims(tokenStr, &rawClaims{},
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(_jwtSecret), nil
		})
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*rawClaims)
	if !ok || !tok.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return &TokenClaims{
		ExtensionName: claims.Subject,
		Privileges:    claims.Privileges,
	}, nil
}
