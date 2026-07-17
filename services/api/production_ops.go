package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	prod "printstudio/api/production"
)

func productionSceneRender(a *API) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := identity(r)
		var req prod.SceneRenderRequest
		if decode(w, r, &req) != nil {
			return
		}
		img, err := prod.RenderScene(req, a.assetFetcher(r, id.WorkspaceID))
		if err != nil {
			prod.DefaultMetrics.Failures.Add(1)
			problem(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		data, err := prod.EncodeScenePNG(img)
		if err != nil {
			prod.DefaultMetrics.Failures.Add(1)
			problem(w, http.StatusInternalServerError, "scene PNG encode failed")
			return
		}
		prod.DefaultMetrics.SceneRenders.Add(1)
		digest := sha256.Sum256(data)
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("X-PrintStudio-Renderer", "server-scene")
		w.Header().Set("X-PrintStudio-SHA256", hex.EncodeToString(digest[:]))
		w.Header().Set("X-PrintStudio-Width-Px", fmt.Sprintf("%d", img.Bounds().Dx()))
		w.Header().Set("X-PrintStudio-Height-Px", fmt.Sprintf("%d", img.Bounds().Dy()))
		_, _ = w.Write(data)
	}
}

func (a *API) assetFetcher(r *http.Request, workspaceID string) prod.AssetFetcher {
	return func(assetID string) (io.ReadCloser, error) {
		var key, status string
		err := a.db.QueryRowContext(r.Context(), `SELECT object_key,status FROM assets WHERE id=$1 AND workspace_id=$2`, assetID, workspaceID).Scan(&key, &status)
		if err != nil {
			return nil, fmt.Errorf("asset %s not found", assetID)
		}
		// Upload complete marks assets as "validated"; treat that as production-ready.
		if status != "validated" && status != "ready" {
			return nil, fmt.Errorf("asset %s is not ready (status=%s)", assetID, status)
		}
		return a.objects.open(r.Context(), key)
	}
}

func productionGates(w http.ResponseWriter, _ *http.Request) {
	write(w, http.StatusOK, map[string]any{"gates": prod.MethodAcceptanceGates()})
}

func productionMetrics(w http.ResponseWriter, _ *http.Request) {
	caps := prod.NativeTools{Vips: os.Getenv("VIPS_BIN"), Potrace: os.Getenv("POTRACE_BIN")}.Probe()
	caps.MaxRenderPixels = productionMaxPixels()
	caps.ICCProfiles = iccProfiles != nil
	write(w, http.StatusOK, map[string]any{
		"counters":     prod.DefaultMetrics.Snapshot(),
		"capabilities": caps,
		"requireNatives": strings.EqualFold(env("REQUIRE_PRODUCTION_NATIVES", "false"), "true"),
		"requireIcc":     requireICCByPolicy(),
	})
}

func createProductionProof(a *API) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := identity(r)
		var in struct {
			DesignID       string         `json:"designId"`
			DesignVersion  int            `json:"designVersion"`
			Method         string         `json:"method"`
			ArtifactSHA256 string         `json:"artifactSha256"`
			WidthMM        float64        `json:"widthMm"`
			HeightMM       float64        `json:"heightMm"`
			Checklist      map[string]bool `json:"checklist"`
			Notes          string         `json:"notes"`
		}
		if decode(w, r, &in) != nil {
			return
		}
		if in.DesignID == "" || in.DesignVersion < 1 || in.Method == "" || len(in.ArtifactSHA256) != 64 || in.WidthMM <= 0 || in.HeightMM <= 0 {
			problem(w, http.StatusUnprocessableEntity, "designId, designVersion, method, artifactSha256 and physical size are required")
			return
		}
		gate, err := prod.LookupMethodGate(in.Method)
		if err != nil {
			problem(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		caps := prod.NativeTools{Vips: os.Getenv("VIPS_BIN"), Potrace: os.Getenv("POTRACE_BIN")}.Probe()
		caps.ICCProfiles = iccProfiles != nil
		checklist := prod.SatisfySystemChecks(in.Method, in.Checklist, caps)
		for _, check := range gate.Checks {
			if check.Required && !checklist[check.ID] {
				problem(w, http.StatusUnprocessableEntity, "required acceptance check not confirmed: "+check.ID)
				return
			}
		}
		var exists int
		if err := a.db.QueryRowContext(r.Context(), `SELECT 1 FROM designs WHERE id=$1 AND workspace_id=$2 AND deleted_at IS NULL`, in.DesignID, id.WorkspaceID).Scan(&exists); err != nil {
			problem(w, http.StatusNotFound, "design not found")
			return
		}
		raw, _ := json.Marshal(checklist)
		var proofID string
		err = a.db.QueryRowContext(r.Context(), `INSERT INTO production_proofs(workspace_id,design_id,design_version,method,artifact_sha256,width_mm,height_mm,checklist,status,created_by,notes)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,'pending',$9,$10) RETURNING id`,
			id.WorkspaceID, in.DesignID, in.DesignVersion, in.Method, strings.ToLower(in.ArtifactSHA256), in.WidthMM, in.HeightMM, raw, id.UserID, strings.TrimSpace(in.Notes)).Scan(&proofID)
		if err != nil {
			problem(w, http.StatusInternalServerError, "could not create production proof")
			return
		}
		a.audit(r, "production.proof_created", proofID)
		write(w, http.StatusCreated, map[string]any{"id": proofID, "status": "pending", "method": in.Method, "designVersion": in.DesignVersion})
	}
}

func approveProductionProof(a *API) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := identity(r)
		proofID := r.PathValue("id")
		var status, method string
		var designID string
		var version int
		err := a.db.QueryRowContext(r.Context(), `SELECT status,method,design_id,design_version FROM production_proofs WHERE id=$1 AND workspace_id=$2`, proofID, id.WorkspaceID).Scan(&status, &method, &designID, &version)
		if err == sql.ErrNoRows {
			problem(w, http.StatusNotFound, "proof not found")
			return
		}
		if err != nil {
			problem(w, http.StatusInternalServerError, "proof lookup failed")
			return
		}
		if status != "pending" {
			problem(w, http.StatusConflict, "proof is already "+status)
			return
		}
		_, err = a.db.ExecContext(r.Context(), `UPDATE production_proofs SET status='approved', approved_by=$1, approved_at=$2 WHERE id=$3`, id.UserID, time.Now().UTC(), proofID)
		if err != nil {
			problem(w, http.StatusInternalServerError, "proof approval failed")
			return
		}
		prod.DefaultMetrics.Approvals.Add(1)
		a.audit(r, "production.proof_approved", proofID)
		write(w, http.StatusOK, map[string]any{
			"id": proofID, "status": "approved", "method": method, "designId": designID, "designVersion": version,
			"frozen": true, "message": "Design version is frozen for fulfilment against this proof",
		})
	}
}

func getProductionProof(a *API) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := identity(r)
		var proof struct {
			ID             string          `json:"id"`
			DesignID       string          `json:"designId"`
			DesignVersion  int             `json:"designVersion"`
			Method         string          `json:"method"`
			ArtifactSHA256 string          `json:"artifactSha256"`
			WidthMM        float64         `json:"widthMm"`
			HeightMM       float64         `json:"heightMm"`
			Checklist      json.RawMessage `json:"checklist"`
			Status         string          `json:"status"`
			CreatedAt      time.Time       `json:"createdAt"`
			ApprovedAt     *time.Time      `json:"approvedAt,omitempty"`
			Notes          string          `json:"notes"`
		}
		err := a.db.QueryRowContext(r.Context(), `SELECT id,design_id,design_version,method,artifact_sha256,width_mm,height_mm,checklist,status,created_at,approved_at,notes
			FROM production_proofs WHERE id=$1 AND workspace_id=$2`, r.PathValue("id"), id.WorkspaceID).
			Scan(&proof.ID, &proof.DesignID, &proof.DesignVersion, &proof.Method, &proof.ArtifactSHA256, &proof.WidthMM, &proof.HeightMM, &proof.Checklist, &proof.Status, &proof.CreatedAt, &proof.ApprovedAt, &proof.Notes)
		if err == sql.ErrNoRows {
			problem(w, http.StatusNotFound, "proof not found")
			return
		}
		if err != nil {
			problem(w, http.StatusInternalServerError, "proof lookup failed")
			return
		}
		write(w, http.StatusOK, proof)
	}
}

func requireApprovedProof(a *API, r *http.Request, proofID, expectedSHA string) error {
	if !strings.EqualFold(env("REQUIRE_PRODUCTION_APPROVAL", "false"), "true") {
		return nil
	}
	if proofID == "" {
		return fmt.Errorf("approved production proof id is required before packaging")
	}
	id := identity(r)
	var status, sha string
	err := a.db.QueryRowContext(r.Context(), `SELECT status,artifact_sha256 FROM production_proofs WHERE id=$1 AND workspace_id=$2`, proofID, id.WorkspaceID).Scan(&status, &sha)
	if err != nil {
		return fmt.Errorf("production proof not found")
	}
	if status != "approved" {
		return fmt.Errorf("production proof must be approved before packaging")
	}
	if expectedSHA != "" && !strings.EqualFold(sha, expectedSHA) {
		return fmt.Errorf("production proof artifact hash does not match the packaged artwork")
	}
	return nil
}

func requireICCByPolicy() bool {
	return strings.EqualFold(env("REQUIRE_ICC", "false"), "true")
}

func enforceProductionNativesOrExit() {
	if !strings.EqualFold(env("REQUIRE_PRODUCTION_NATIVES", "false"), "true") {
		return
	}
	caps := prod.NativeTools{Vips: os.Getenv("VIPS_BIN"), Potrace: os.Getenv("POTRACE_BIN")}.Probe()
	missing := make([]string, 0, 4)
	if !caps.ICC {
		missing = append(missing, "libvips/LittleCMS")
	}
	if !caps.VectorTrace {
		missing = append(missing, "potrace")
	}
	if !caps.PolygonBoolean {
		missing = append(missing, "Clipper2 (-tags clipper2)")
	}
	if iccProfiles == nil {
		missing = append(missing, "ICC_PROFILE_DIR")
	}
	if len(missing) > 0 {
		log.Fatalf("REQUIRE_PRODUCTION_NATIVES=true but missing: %s", strings.Join(missing, ", "))
	}
	log.Printf("production natives ready: icc=%v clipper2=%v potrace=%v profiles=%v", caps.ICC, caps.PolygonBoolean, caps.VectorTrace, iccProfiles != nil)
}
