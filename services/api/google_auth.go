package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"google.golang.org/api/idtoken"
)

type googleLoginRequest struct {
	IDToken string `json:"idToken"`
}

type sessionResponse struct {
	AccessToken string    `json:"accessToken"`
	TokenType   string    `json:"tokenType"`
	ExpiresAt   time.Time `json:"expiresAt"`
	ExpiresIn   int       `json:"expiresIn"`
	User        sessionUser `json:"user"`
}

type sessionUser struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	Role        string `json:"role"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}

func (a *API) loginGoogle(w http.ResponseWriter, r *http.Request) {
	if authMode() != "jwt" {
		problem(w, http.StatusBadRequest, "Google sign-in requires AUTH_MODE=jwt")
		return
	}
	var in googleLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.IDToken) == "" {
		problem(w, http.StatusBadRequest, "idToken is required")
		return
	}
	id, profile, err := a.provisionGoogleIdentity(r.Context(), in.IDToken)
	if err != nil {
		problem(w, http.StatusUnauthorized, err.Error())
		return
	}
	token, expiresAt, err := issueSessionJWT(id)
	if err != nil {
		problem(w, http.StatusInternalServerError, "could not issue session")
		return
	}
	setSessionCookie(w, token, expiresAt)
	write(w, http.StatusOK, sessionResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt,
		ExpiresIn:   int(time.Until(expiresAt).Seconds()),
		User: sessionUser{
			ID: id.UserID, WorkspaceID: id.WorkspaceID, Role: id.Role,
			Email: profile.email, DisplayName: profile.name,
		},
	})
}

func (a *API) authMe(w http.ResponseWriter, r *http.Request) {
	id := identity(r)
	var email, name string
	_ = a.db.QueryRowContext(r.Context(), `SELECT email,display_name FROM users WHERE id=$1`, id.UserID).Scan(&email, &name)
	write(w, http.StatusOK, sessionUser{ID: id.UserID, WorkspaceID: id.WorkspaceID, Role: id.Role, Email: email, DisplayName: name})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	clearSessionCookie(w)
	write(w, http.StatusOK, map[string]any{"ok": true})
}

type googleProfile struct {
	sub, email, name, picture string
}

func (a *API) provisionGoogleIdentity(ctx context.Context, rawToken string) (Identity, googleProfile, error) {
	audience := env("GOOGLE_CLIENT_ID", "")
	payload, err := idtoken.Validate(ctx, rawToken, audience)
	if err != nil {
		return Identity{}, googleProfile{}, errors.New("invalid Google ID token")
	}
	sub, _ := payload.Claims["sub"].(string)
	email, _ := payload.Claims["email"].(string)
	name, _ := payload.Claims["name"].(string)
	picture, _ := payload.Claims["picture"].(string)
	verified, _ := payload.Claims["email_verified"].(bool)
	if sub == "" || email == "" || !verified {
		return Identity{}, googleProfile{}, errors.New("verified Google identity required")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if strings.TrimSpace(name) == "" {
		name = strings.Split(email, "@")[0]
	}
	profile := googleProfile{sub: sub, email: email, name: name, picture: picture}

	var out Identity
	err = a.db.QueryRowContext(ctx, `
		SELECT u.id,m.workspace_id,m.role
		FROM users u JOIN memberships m ON m.user_id=u.id
		WHERE u.external_subject=$1
		ORDER BY CASE m.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END, m.workspace_id
		LIMIT 1`, sub).Scan(&out.UserID, &out.WorkspaceID, &out.Role)
	if err == nil {
		return out, profile, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Identity{}, profile, err
	}

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return Identity{}, profile, err
	}
	defer tx.Rollback()

	err = tx.QueryRowContext(ctx, `
		INSERT INTO users(id,email,display_name,external_subject,avatar_url)
		VALUES(gen_random_uuid(),$1,$2,$3,$4)
		ON CONFLICT(email) DO UPDATE SET
			external_subject=EXCLUDED.external_subject,
			display_name=EXCLUDED.display_name,
			avatar_url=EXCLUDED.avatar_url
		RETURNING id`, email, name, sub, picture).Scan(&out.UserID)
	if err != nil {
		return Identity{}, profile, err
	}

	err = tx.QueryRowContext(ctx, `
		SELECT workspace_id,role FROM memberships WHERE user_id=$1
		ORDER BY CASE role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END, workspace_id
		LIMIT 1`, out.UserID).Scan(&out.WorkspaceID, &out.Role)
	if err == nil {
		if err = tx.Commit(); err != nil {
			return Identity{}, profile, err
		}
		return out, profile, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Identity{}, profile, err
	}

	// Accept the oldest pending invite for this email before creating a personal workspace.
	var inviteID string
	err = tx.QueryRowContext(ctx, `
		SELECT id::text,workspace_id,role FROM invites
		WHERE lower(email)=$1 AND accepted_at IS NULL
		ORDER BY created_at ASC LIMIT 1`, email).Scan(&inviteID, &out.WorkspaceID, &out.Role)
	if err == nil {
		if _, err = tx.ExecContext(ctx, `INSERT INTO memberships(workspace_id,user_id,role) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, out.WorkspaceID, out.UserID, out.Role); err != nil {
			return Identity{}, profile, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE invites SET accepted_at=now() WHERE id::text=$1`, inviteID); err != nil {
			return Identity{}, profile, err
		}
		if err = tx.Commit(); err != nil {
			return Identity{}, profile, err
		}
		return out, profile, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Identity{}, profile, err
	}

	err = tx.QueryRowContext(ctx, `INSERT INTO workspaces(id,name) VALUES(gen_random_uuid(),$1) RETURNING id`, name+"'s Workspace").Scan(&out.WorkspaceID)
	if err != nil {
		return Identity{}, profile, err
	}
	out.Role = "owner"
	if _, err = tx.ExecContext(ctx, `INSERT INTO memberships(workspace_id,user_id,role) VALUES($1,$2,'owner') ON CONFLICT DO NOTHING`, out.WorkspaceID, out.UserID); err != nil {
		return Identity{}, profile, err
	}
	// Re-read in case of concurrent insert.
	err = tx.QueryRowContext(ctx, `
		SELECT workspace_id,role FROM memberships WHERE user_id=$1
		ORDER BY CASE role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END, workspace_id
		LIMIT 1`, out.UserID).Scan(&out.WorkspaceID, &out.Role)
	if err != nil {
		return Identity{}, profile, err
	}
	if err = tx.Commit(); err != nil {
		return Identity{}, profile, err
	}
	return out, profile, nil
}

func setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	secure := strings.HasPrefix(strings.ToLower(env("WEB_ORIGIN", "http://localhost:3000")), "https://")
	sameSite := http.SameSiteLaxMode
	if secure {
		sameSite = http.SameSiteNoneMode
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	secure := strings.HasPrefix(strings.ToLower(env("WEB_ORIGIN", "http://localhost:3000")), "https://")
	sameSite := http.SameSiteLaxMode
	if secure {
		sameSite = http.SameSiteNoneMode
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}
