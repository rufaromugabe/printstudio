package main

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"google.golang.org/api/idtoken"
)

func (a *API) googleIdentity(ctx context.Context, token string) (Identity, error) {
	payload, err := idtoken.Validate(ctx, token, env("GOOGLE_CLIENT_ID", ""))
	if err != nil {
		return Identity{}, errors.New("invalid Google ID token")
	}
	sub, _ := payload.Claims["sub"].(string)
	email, _ := payload.Claims["email"].(string)
	name, _ := payload.Claims["name"].(string)
	picture, _ := payload.Claims["picture"].(string)
	verified, _ := payload.Claims["email_verified"].(bool)
	if sub == "" || email == "" || !verified {
		return Identity{}, errors.New("verified Google identity required")
	}
	var out Identity
	err = a.db.QueryRowContext(ctx, `SELECT u.id,m.workspace_id,m.role FROM users u JOIN memberships m ON m.user_id=u.id WHERE u.external_subject=$1 ORDER BY m.role='owner' DESC LIMIT 1`, sub).Scan(&out.UserID, &out.WorkspaceID, &out.Role)
	if err == nil {
		return out, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Identity{}, err
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return Identity{}, err
	}
	defer tx.Rollback()
	if strings.TrimSpace(name) == "" {
		name = strings.Split(email, "@")[0]
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO users(id,email,display_name,external_subject,avatar_url) VALUES(gen_random_uuid(),$1,$2,$3,$4) ON CONFLICT(email) DO UPDATE SET external_subject=EXCLUDED.external_subject,display_name=EXCLUDED.display_name,avatar_url=EXCLUDED.avatar_url RETURNING id`, strings.ToLower(email), name, sub, picture).Scan(&out.UserID)
	if err != nil {
		return Identity{}, err
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO workspaces(id,name) VALUES(gen_random_uuid(),$1) RETURNING id`, name+"'s Workspace").Scan(&out.WorkspaceID)
	if err != nil {
		return Identity{}, err
	}
	out.Role = "owner"
	if _, err = tx.ExecContext(ctx, `INSERT INTO memberships(workspace_id,user_id,role) VALUES($1,$2,'owner')`, out.WorkspaceID, out.UserID); err != nil {
		return Identity{}, err
	}
	if err = tx.Commit(); err != nil {
		return Identity{}, err
	}
	return out, nil
}
