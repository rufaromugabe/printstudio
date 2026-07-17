package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

type MembershipRow struct {
	UserID      string    `json:"userId"`
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
	Role        string    `json:"role"`
	Kind        string    `json:"kind"` // member | invite
	CreatedAt   time.Time `json:"createdAt,omitempty"`
}

func validAssignableRole(role string) bool {
	switch role {
	case "admin", "member", "viewer":
		return true
	default:
		return false
	}
}

func (a *API) listMemberships(w http.ResponseWriter, r *http.Request) {
	id := identity(r)
	out := []MembershipRow{}
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT u.id,u.email,COALESCE(u.display_name,''),m.role,'member',COALESCE(u.created_at, now())
		FROM memberships m JOIN users u ON u.id=m.user_id
		WHERE m.workspace_id=$1
		ORDER BY CASE m.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 WHEN 'member' THEN 2 ELSE 3 END, u.email`, id.WorkspaceID)
	if err != nil {
		problem(w, 500, "query failed")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var row MembershipRow
		if rows.Scan(&row.UserID, &row.Email, &row.DisplayName, &row.Role, &row.Kind, &row.CreatedAt) == nil {
			out = append(out, row)
		}
	}
	invites, err := a.db.QueryContext(r.Context(), `
		SELECT id::text,email,'',role,'invite',created_at
		FROM invites WHERE workspace_id=$1 AND accepted_at IS NULL ORDER BY created_at DESC`, id.WorkspaceID)
	if err == nil {
		defer invites.Close()
		for invites.Next() {
			var row MembershipRow
			if invites.Scan(&row.UserID, &row.Email, &row.DisplayName, &row.Role, &row.Kind, &row.CreatedAt) == nil {
				out = append(out, row)
			}
		}
	}
	write(w, 200, out)
}

func (a *API) inviteMembership(w http.ResponseWriter, r *http.Request) {
	id := identity(r)
	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		problem(w, 400, "invalid body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	role := strings.TrimSpace(body.Role)
	if email == "" || !strings.Contains(email, "@") || !validAssignableRole(role) {
		problem(w, 422, "email and role (admin|member|viewer) are required")
		return
	}

	var existingUser string
	err := a.db.QueryRowContext(r.Context(), `SELECT id FROM users WHERE lower(email)=$1`, email).Scan(&existingUser)
	if err == nil {
		var currentRole string
		err = a.db.QueryRowContext(r.Context(), `SELECT role FROM memberships WHERE workspace_id=$1 AND user_id=$2`, id.WorkspaceID, existingUser).Scan(&currentRole)
		if err == nil {
			if currentRole == "owner" {
				problem(w, 422, "cannot change the workspace owner through invite")
				return
			}
			if _, err = a.db.ExecContext(r.Context(), `UPDATE memberships SET role=$3 WHERE workspace_id=$1 AND user_id=$2`, id.WorkspaceID, existingUser, role); err != nil {
				problem(w, 500, "update failed")
				return
			}
			write(w, 200, map[string]any{"userId": existingUser, "email": email, "role": role, "kind": "member"})
			return
		}
		if !errors.Is(err, sql.ErrNoRows) {
			problem(w, 500, "query failed")
			return
		}
		if _, err = a.db.ExecContext(r.Context(), `INSERT INTO memberships(workspace_id,user_id,role) VALUES($1,$2,$3)`, id.WorkspaceID, existingUser, role); err != nil {
			problem(w, 500, "insert failed")
			return
		}
		write(w, 201, map[string]any{"userId": existingUser, "email": email, "role": role, "kind": "member"})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		problem(w, 500, "query failed")
		return
	}

	var inviteID string
	err = a.db.QueryRowContext(r.Context(), `
		INSERT INTO invites(workspace_id,email,role,invited_by)
		VALUES($1,$2,$3,$4)
		ON CONFLICT(workspace_id,email) DO UPDATE SET role=EXCLUDED.role, invited_by=EXCLUDED.invited_by, accepted_at=NULL, created_at=now()
		RETURNING id::text`, id.WorkspaceID, email, role, id.UserID).Scan(&inviteID)
	if err != nil {
		problem(w, 500, "invite failed")
		return
	}
	write(w, 201, map[string]any{"userId": inviteID, "email": email, "role": role, "kind": "invite"})
}

func (a *API) updateMembership(w http.ResponseWriter, r *http.Request) {
	id := identity(r)
	userID := r.PathValue("userId")
	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		problem(w, 400, "invalid body")
		return
	}
	role := strings.TrimSpace(body.Role)
	if !validAssignableRole(role) {
		problem(w, 422, "role must be admin, member, or viewer")
		return
	}
	var current string
	err := a.db.QueryRowContext(r.Context(), `SELECT role FROM memberships WHERE workspace_id=$1 AND user_id=$2`, id.WorkspaceID, userID).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		problem(w, 404, "membership not found")
		return
	}
	if err != nil {
		problem(w, 500, "query failed")
		return
	}
	if current == "owner" {
		problem(w, 422, "cannot change the owner role")
		return
	}
	if userID == id.UserID && id.Role != "owner" {
		problem(w, 422, "admins cannot demote themselves")
		return
	}
	if _, err = a.db.ExecContext(r.Context(), `UPDATE memberships SET role=$3 WHERE workspace_id=$1 AND user_id=$2`, id.WorkspaceID, userID, role); err != nil {
		problem(w, 500, "update failed")
		return
	}
	write(w, 200, map[string]any{"userId": userID, "role": role})
}

func (a *API) removeMembership(w http.ResponseWriter, r *http.Request) {
	id := identity(r)
	userID := r.PathValue("userId")
	var current string
	err := a.db.QueryRowContext(r.Context(), `SELECT role FROM memberships WHERE workspace_id=$1 AND user_id=$2`, id.WorkspaceID, userID).Scan(&current)
	if err == nil {
		if current == "owner" {
			problem(w, 422, "cannot remove the workspace owner")
			return
		}
		if userID == id.UserID {
			problem(w, 422, "cannot remove yourself")
			return
		}
		if _, err = a.db.ExecContext(r.Context(), `DELETE FROM memberships WHERE workspace_id=$1 AND user_id=$2`, id.WorkspaceID, userID); err != nil {
			problem(w, 500, "delete failed")
			return
		}
		write(w, 200, map[string]any{"removed": userID, "kind": "member"})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		problem(w, 500, "query failed")
		return
	}
	res, err := a.db.ExecContext(r.Context(), `DELETE FROM invites WHERE workspace_id=$1 AND id::text=$2 AND accepted_at IS NULL`, id.WorkspaceID, userID)
	if err != nil {
		problem(w, 500, "delete failed")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		problem(w, 404, "membership or invite not found")
		return
	}
	write(w, 200, map[string]any{"removed": userID, "kind": "invite"})
}

func (a *API) requireNotViewer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if identity(r).Role == "viewer" {
			problem(w, 403, "viewers cannot modify workspace data")
			return
		}
		next.ServeHTTP(w, r)
	})
}
