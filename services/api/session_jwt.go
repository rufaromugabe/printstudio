package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	sessionCookieName = "printstudio_session"
	sessionIssuer     = "printstudio"
	sessionTTL        = time.Hour
)

type sessionClaims struct {
	Iss string `json:"iss"`
	Sub string `json:"sub"`
	Wid string `json:"wid"`
	Rol string `json:"rol"`
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
}

func authMode() string {
	return strings.ToLower(strings.TrimSpace(env("AUTH_MODE", "dev")))
}

func jwtSecret() ([]byte, error) {
	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if secret == "" {
		return nil, errors.New("JWT_SECRET is required when AUTH_MODE is jwt")
	}
	if len(secret) < 32 {
		return nil, errors.New("JWT_SECRET must be at least 32 characters")
	}
	return []byte(secret), nil
}

func issueSessionJWT(id Identity) (token string, expiresAt time.Time, err error) {
	secret, err := jwtSecret()
	if err != nil {
		return "", time.Time{}, err
	}
	now := time.Now().UTC()
	expiresAt = now.Add(sessionTTL)
	claims := sessionClaims{
		Iss: sessionIssuer,
		Sub: id.UserID,
		Wid: id.WorkspaceID,
		Rol: id.Role,
		Iat: now.Unix(),
		Exp: expiresAt.Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	body := base64.RawURLEncoding.EncodeToString(payload)
	signingInput := header + "." + body
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(signingInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + sig, expiresAt, nil
}

func parseSessionJWT(token string) (Identity, error) {
	secret, err := jwtSecret()
	if err != nil {
		return Identity{}, err
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Identity{}, errors.New("malformed session token")
	}
	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(signingInput))
	expected := mac.Sum(nil)
	got, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(got, expected) {
		return Identity{}, errors.New("invalid session signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Identity{}, errors.New("invalid session payload")
	}
	var claims sessionClaims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return Identity{}, errors.New("invalid session claims")
	}
	now := time.Now().UTC().Unix()
	if claims.Iss != sessionIssuer || claims.Sub == "" || claims.Wid == "" || claims.Rol == "" {
		return Identity{}, errors.New("incomplete session claims")
	}
	if claims.Exp < now || claims.Iat > now+60 {
		return Identity{}, errors.New("session expired")
	}
	return Identity{UserID: claims.Sub, WorkspaceID: claims.Wid, Role: claims.Rol}, nil
}

func bearerToken(rHeader string) string {
	token := strings.TrimSpace(rHeader)
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		return strings.TrimSpace(token[7:])
	}
	return token
}

func extractSessionToken(authorization string, cookieValue string) string {
	if tok := bearerToken(authorization); tok != "" {
		return tok
	}
	return strings.TrimSpace(cookieValue)
}

func mustConfigureAuth() error {
	switch authMode() {
	case "dev":
		return nil
	case "jwt":
		if _, err := jwtSecret(); err != nil {
			return err
		}
		if strings.TrimSpace(env("GOOGLE_CLIENT_ID", "")) == "" {
			return fmt.Errorf("GOOGLE_CLIENT_ID is required when AUTH_MODE is jwt")
		}
		return nil
	default:
		return fmt.Errorf("unsupported AUTH_MODE %q; use dev or jwt", authMode())
	}
}
