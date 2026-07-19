package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	prod "printstudio/api/production"
)

//go:embed migrations/*.sql
var migrations embed.FS

type ctxKey string

const identityKey ctxKey = "identity"

type Identity struct{ UserID, WorkspaceID, Role string }
type Design struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspaceId,omitempty"`
	Name        string          `json:"name"`
	Document    json.RawMessage `json:"document"`
	Version     int             `json:"version"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}
type Product struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Methods  []string        `json:"methods"`
	Views    []string        `json:"views"`
	Active   bool            `json:"active"`
	Template json.RawMessage `json:"template"`
}
type API struct {
	db         *sql.DB
	maxDesigns int
	objects    *ObjectStore
}

func main() {
	loadDotEnv()
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		resp, err := http.Get("http://127.0.0.1:" + env("PORT", "8080") + "/health/ready")
		if err != nil || resp.StatusCode != 200 {
			os.Exit(1)
		}
		return
	}
	dsn := env("DATABASE_URL", "postgres://printstudio:printstudio@localhost:5432/printstudio?sslmode=disable")
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		log.Fatalf("database unavailable: %v", err)
	}
	if err = runMigrations(ctx, db); err != nil {
		log.Fatalf("migrations: %v", err)
	}
	objects := newObjectStore()
	storeCtx, storeCancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer storeCancel()
	if err = objects.ensureBucket(storeCtx); err != nil {
		log.Fatalf("object storage unavailable: %v", err)
	}
	api := &API{db: db, maxDesigns: 100, objects: objects}
	if dir := strings.TrimSpace(os.Getenv("ICC_PROFILE_DIR")); dir != "" {
		store, err := prod.NewICCProfileStore(dir)
		if err != nil {
			log.Fatalf("ICC profile store: %v", err)
		}
		if err := prod.SeedCommonICCProfiles(store); err != nil {
			log.Fatalf("ICC common profile seed: %v", err)
		}
		iccProfiles = store
		log.Printf("seeded common ICC profiles into %s", dir)
	}
	enforceProductionNativesOrExit()
	if err := mustConfigureAuth(); err != nil {
		log.Fatalf("auth configuration: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) { write(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /health/ready", api.ready)
	mux.HandleFunc("POST /v1/auth/google", api.loginGoogle)
	mux.Handle("GET /v1/auth/me", api.auth(http.HandlerFunc(api.authMe)))
	mux.HandleFunc("POST /v1/auth/logout", api.logout)
	mux.Handle("GET /v1/memberships", api.auth(api.requireRole("admin", http.HandlerFunc(api.listMemberships))))
	mux.Handle("POST /v1/memberships", api.auth(api.requireRole("admin", http.HandlerFunc(api.inviteMembership))))
	mux.Handle("PATCH /v1/memberships/{userId}", api.auth(api.requireRole("admin", http.HandlerFunc(api.updateMembership))))
	mux.Handle("DELETE /v1/memberships/{userId}", api.auth(api.requireRole("admin", http.HandlerFunc(api.removeMembership))))
	mux.Handle("GET /v1/products", api.auth(http.HandlerFunc(api.listProducts)))
	mux.Handle("POST /v1/products", api.auth(api.requireRole("admin", http.HandlerFunc(api.upsertProduct))))
	mux.Handle("GET /v1/designs", api.auth(http.HandlerFunc(api.listDesigns)))
	mux.Handle("POST /v1/designs", api.auth(api.requireNotViewer(http.HandlerFunc(api.createDesign))))
	mux.Handle("GET /v1/designs/{id}", api.auth(http.HandlerFunc(api.getDesign)))
	mux.Handle("PUT /v1/designs/{id}", api.auth(api.requireNotViewer(http.HandlerFunc(api.updateDesign))))
	mux.Handle("GET /v1/designs/{id}/versions", api.auth(http.HandlerFunc(api.listVersions)))
	mux.Handle("POST /v1/designs/{id}/shares", api.auth(api.requireNotViewer(http.HandlerFunc(api.createShare))))
	mux.Handle("GET /v1/designs/{id}/shares", api.auth(http.HandlerFunc(api.listShares)))
	mux.Handle("DELETE /v1/designs/{id}/shares/{token}", api.auth(api.requireNotViewer(http.HandlerFunc(api.revokeShare))))
	mux.HandleFunc("GET /v1/shared/{token}", api.getShared)
	mux.Handle("GET /v1/audit", api.auth(api.requireRole("admin", http.HandlerFunc(api.listAudit))))
	mux.Handle("GET /v1/assets", api.auth(http.HandlerFunc(api.listAssets)))
	mux.Handle("POST /v1/assets/uploads", api.auth(api.requireNotViewer(http.HandlerFunc(api.createAssetUpload))))
	mux.Handle("POST /v1/assets/{id}/complete", api.auth(api.requireNotViewer(http.HandlerFunc(api.completeAssetUpload))))
	mux.Handle("GET /v1/assets/{id}/url", api.auth(http.HandlerFunc(api.assetURL)))
	mux.Handle("POST /v1/embroidery/compile", api.auth(api.requireNotViewer(http.HandlerFunc(compileEmbroidery))))
	mux.Handle("POST /v1/embroidery/export/dst", api.auth(api.requireNotViewer(http.HandlerFunc(exportEmbroidery))))
	mux.Handle("POST /v1/vinyl/review", api.auth(api.requireNotViewer(http.HandlerFunc(reviewVinyl))))
	mux.Handle("GET /v1/production/capabilities", api.auth(http.HandlerFunc(productionCapabilities)))
	mux.Handle("GET /v1/production/gates", api.auth(http.HandlerFunc(productionGates)))
	mux.Handle("GET /v1/production/metrics", api.auth(api.requireRole("admin", http.HandlerFunc(productionMetrics))))
	mux.Handle("POST /v1/production/render/scene", api.auth(productionSceneRender(api)))
	mux.Handle("POST /v1/production/proofs", api.auth(createProductionProof(api)))
	mux.Handle("GET /v1/production/proofs/{id}", api.auth(getProductionProof(api)))
	mux.Handle("POST /v1/production/proofs/{id}/approve", api.auth(approveProductionProof(api)))
	mux.Handle("POST /v1/production/dtf/underbase", api.auth(http.HandlerFunc(productionUnderbase)))
	mux.Handle("POST /v1/production/dtf/pack", api.auth(api.requireNotViewer(productionDTFPackHandler(api))))
	mux.Handle("POST /v1/production/sublimation/pack", api.auth(api.requireNotViewer(productionSublimationPackHandler(api))))
	mux.Handle("POST /v1/production/screen/halftone", api.auth(api.requireNotViewer(http.HandlerFunc(productionHalftone))))
	mux.Handle("POST /v1/production/screen/cmyk", api.auth(api.requireNotViewer(http.HandlerFunc(productionCMYK))))
	mux.Handle("POST /v1/production/screen/pack", api.auth(api.requireNotViewer(productionScreenPackHandler(api))))
	mux.Handle("POST /v1/production/screen/angles", api.auth(http.HandlerFunc(productionAngleCheck)))
	mux.Handle("POST /v1/production/spot/match", api.auth(http.HandlerFunc(productionSpotMatch)))
	mux.Handle("GET /v1/production/icc/profiles", api.auth(http.HandlerFunc(listICCProfiles)))
	mux.Handle("POST /v1/production/icc/profiles", api.auth(api.requireRole("admin", http.HandlerFunc(uploadICCProfile))))
	mux.Handle("POST /v1/production/icc/transform", api.auth(http.HandlerFunc(applyICCTransform)))
	mux.Handle("POST /v1/production/gang/nest", api.auth(http.HandlerFunc(productionNest)))
	mux.Handle("POST /v1/production/gang/render", api.auth(productionGangRenderHandler(api)))
	mux.Handle("POST /v1/production/vector/boolean", api.auth(productionBooleanHandler()))
	mux.Handle("POST /v1/production/vector/offset", api.auth(productionOffsetHandler()))
	mux.Handle("POST /v1/production/vectorize", api.auth(api.requireNotViewer(productionVectorizeHandler(api))))
	server := &http.Server{Addr: ":" + env("PORT", "8080"), Handler: requestLog(cors(mux)), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 2 * time.Minute, WriteTimeout: 5 * time.Minute, IdleTimeout: 60 * time.Second}
	go func() {
		log.Printf("PrintStudio API %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdown, c := context.WithTimeout(context.Background(), 10*time.Second)
	defer c()
	_ = server.Shutdown(shutdown)
	_ = db.Close()
}

func runMigrations(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(name text PRIMARY KEY,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, _ := migrations.ReadDir("migrations")
	for _, e := range entries {
		b, err := migrations.ReadFile("migrations/" + e.Name())
		if err != nil {
			return err
		}
		checksum := fmt.Sprintf("%x", sha256.Sum256(b))
		var existing string
		scanErr := db.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE name=$1`, e.Name()).Scan(&existing)
		if scanErr == nil {
			if existing != checksum {
				return fmt.Errorf("migration %s checksum changed", e.Name())
			}
			continue
		}
		if !errors.Is(scanErr, sql.ErrNoRows) {
			return scanErr
		}
		tx, txErr := db.BeginTx(ctx, nil)
		if txErr != nil {
			return txErr
		}
		if _, err = tx.ExecContext(ctx, string(b)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(name,checksum) VALUES($1,$2)`, e.Name(), checksum); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	ctx, c := context.WithTimeout(r.Context(), 2*time.Second)
	defer c()
	if a.db.PingContext(ctx) != nil {
		problem(w, 503, "database unavailable")
		return
	}
	caps := prod.NativeTools{Vips: os.Getenv("VIPS_BIN"), Potrace: os.Getenv("POTRACE_BIN")}.Probe()
	write(w, 200, map[string]any{
		"status":           "ready",
		"icc":              caps.ICC,
		"vectorTrace":      caps.VectorTrace,
		"polygonBoolean":   caps.PolygonBoolean,
		"iccProfiles":      iccProfiles != nil,
		"productionReady":  caps.ICC && caps.VectorTrace && caps.PolygonBoolean && iccProfiles != nil,
		"requireNatives":   strings.EqualFold(env("REQUIRE_PRODUCTION_NATIVES", "false"), "true"),
	})
}

func (a *API) createAssetUpload(w http.ResponseWriter, r *http.Request) {
	id := identity(r)
	var in AssetRequest
	if decode(w, r, &in) != nil {
		return
	}
	if err := validateAssetRequest(in); err != nil {
		problem(w, 422, err.Error())
		return
	}
	in.FileName = cleanFileName(in.FileName)
	var count int
	var used int64
	_ = a.db.QueryRowContext(r.Context(), `SELECT count(*),COALESCE(sum(declared_size),0) FROM assets WHERE workspace_id=$1 AND status<>'rejected'`, id.WorkspaceID).Scan(&count, &used)
	if count >= 500 {
		problem(w, 429, "workspace asset quota reached")
		return
	}
	if used+in.Size > 2<<30 {
		problem(w, 429, "workspace storage quota reached")
		return
	}
	var assetID, key string
	err := a.db.QueryRowContext(r.Context(), `SELECT gen_random_uuid()`).Scan(&assetID)
	if err != nil {
		problem(w, 500, "asset creation failed")
		return
	}
	key = id.WorkspaceID + "/" + assetID + "/" + in.FileName
	_, err = a.db.ExecContext(r.Context(), `INSERT INTO assets(id,workspace_id,owner_id,file_name,object_key,content_type,declared_size,declared_sha256,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'pending')`, assetID, id.WorkspaceID, id.UserID, in.FileName, key, in.ContentType, in.Size, in.SHA256)
	if err != nil {
		problem(w, 500, "asset creation failed")
		return
	}
	url, err := a.objects.uploadURL(r.Context(), key, in.ContentType, in.Size)
	if err != nil {
		problem(w, 503, "upload service unavailable")
		return
	}
	a.audit(r, "asset.upload_created", assetID)
	write(w, 201, map[string]any{"assetId": assetID, "uploadUrl": url, "expiresIn": 900, "requiredHeaders": map[string]any{"Content-Type": in.ContentType, "Content-Length": in.Size}})
}
func (a *API) completeAssetUpload(w http.ResponseWriter, r *http.Request) {
	id := identity(r)
	assetID := r.PathValue("id")
	var key, contentType, checksum, status string
	var size int64
	err := a.db.QueryRowContext(r.Context(), `SELECT object_key,content_type,declared_size,declared_sha256,status FROM assets WHERE id=$1 AND workspace_id=$2`, assetID, id.WorkspaceID).Scan(&key, &contentType, &size, &checksum, &status)
	if errors.Is(err, sql.ErrNoRows) {
		problem(w, 404, "pending asset not found")
		return
	}
	if status == "validated" {
		var v Asset
		err = a.db.QueryRowContext(r.Context(), `SELECT file_name,content_type,actual_size,width,height,status FROM assets WHERE id=$1`, assetID).Scan(&v.FileName, &v.ContentType, &v.Size, &v.Width, &v.Height, &v.Status)
		v.ID = assetID
		v.URL, _ = a.objects.downloadURL(r.Context(), key)
		write(w, 200, v)
		return
	}
	if status != "pending" {
		problem(w, 409, "asset is not pending")
		return
	}
	width, height, validationErr := a.objects.inspect(r.Context(), key, contentType, size, checksum)
	if validationErr != nil {
		_, _ = a.db.ExecContext(r.Context(), `UPDATE assets SET status='rejected',rejection_reason=$1,validated_at=now() WHERE id=$2`, validationErr.Error(), assetID)
		a.objects.delete(r.Context(), key)
		a.audit(r, "asset.rejected", assetID)
		problem(w, 422, validationErr.Error())
		return
	}
	_, err = a.db.ExecContext(r.Context(), `UPDATE assets SET status='validated',actual_size=$1,width=$2,height=$3,validated_at=now() WHERE id=$4 AND workspace_id=$5`, size, width, height, assetID, id.WorkspaceID)
	if err != nil {
		problem(w, 500, "asset completion failed")
		return
	}
	url, _ := a.objects.downloadURL(r.Context(), key)
	a.audit(r, "asset.validated", assetID)
	write(w, 200, Asset{ID: assetID, ContentType: contentType, Size: size, Width: width, Height: height, Status: "validated", URL: url})
}
func (a *API) listAssets(w http.ResponseWriter, r *http.Request) {
	id := identity(r)
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,file_name,content_type,COALESCE(actual_size,declared_size),COALESCE(width,0),COALESCE(height,0),status,COALESCE(rejection_reason,'') FROM assets WHERE workspace_id=$1 ORDER BY created_at DESC LIMIT 100`, id.WorkspaceID)
	if err != nil {
		problem(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []Asset{}
	for rows.Next() {
		var v Asset
		if rows.Scan(&v.ID, &v.FileName, &v.ContentType, &v.Size, &v.Width, &v.Height, &v.Status, &v.RejectionReason) == nil {
			out = append(out, v)
		}
	}
	write(w, 200, out)
}
func (a *API) assetURL(w http.ResponseWriter, r *http.Request) {
	id := identity(r)
	var key string
	err := a.db.QueryRowContext(r.Context(), `SELECT object_key FROM assets WHERE id=$1 AND workspace_id=$2 AND status='validated'`, r.PathValue("id"), id.WorkspaceID).Scan(&key)
	if err != nil {
		problem(w, 404, "validated asset not found")
		return
	}
	url, err := a.objects.downloadURL(r.Context(), key)
	if err != nil {
		problem(w, 503, "download service unavailable")
		return
	}
	write(w, 200, map[string]any{"url": url, "expiresIn": 3600})
}

func (a *API) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch authMode() {
		case "dev":
			id := Identity{env("DEV_USER_ID", "00000000-0000-0000-0000-000000000001"), env("DEV_WORKSPACE_ID", "10000000-0000-0000-0000-000000000001"), r.Header.Get("X-Dev-Role")}
			if id.Role == "" {
				id.Role = "owner"
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey, id)))
			return
		case "jwt":
			cookie, _ := r.Cookie(sessionCookieName)
			cookieVal := ""
			if cookie != nil {
				cookieVal = cookie.Value
			}
			token := extractSessionToken(r.Header.Get("Authorization"), cookieVal)
			id, err := parseSessionJWT(token)
			if err != nil {
				problem(w, http.StatusUnauthorized, "invalid or expired session")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey, id)))
			return
		default:
			problem(w, http.StatusInternalServerError, "unsupported AUTH_MODE; use dev or jwt")
		}
	})
}
func (a *API) requireRole(role string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := identity(r)
		if id.Role != role && id.Role != "owner" {
			problem(w, 403, "insufficient role")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) listProducts(w http.ResponseWriter, r *http.Request) {
	id := identity(r)
	query := `SELECT id,name,methods,views,active,template FROM products WHERE active=true ORDER BY name`
	if id.Role == "owner" || id.Role == "admin" {
		query = `SELECT id,name,methods,views,active,template FROM products ORDER BY name`
	}
	rows, err := a.db.QueryContext(r.Context(), query)
	if err != nil {
		problem(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []Product{}
	for rows.Next() {
		var p Product
		if rows.Scan(&p.ID, &p.Name, &p.Methods, &p.Views, &p.Active, &p.Template) == nil {
			out = append(out, p)
		}
	}
	write(w, 200, out)
}
func (a *API) upsertProduct(w http.ResponseWriter, r *http.Request) {
	var p Product
	if decode(w, r, &p) != nil {
		return
	}
	if p.ID == "" || p.Name == "" {
		problem(w, 422, "id and name are required")
		return
	}
	if err := validateProductTemplate(p.Template); err != nil {
		problem(w, 422, err.Error())
		return
	}
	_, err := a.db.ExecContext(r.Context(), `INSERT INTO products(id,name,methods,views,active,template) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(id) DO UPDATE SET name=$2,methods=$3,views=$4,active=$5,template=$6,updated_at=now()`, p.ID, p.Name, p.Methods, p.Views, p.Active, p.Template)
	if err != nil {
		problem(w, 500, "save failed")
		return
	}
	a.audit(r, "product.upsert", p.ID)
	write(w, 200, p)
}
func (a *API) listDesigns(w http.ResponseWriter, r *http.Request) {
	id := identity(r)
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,name,document,current_version,updated_at FROM designs WHERE workspace_id=$1 AND deleted_at IS NULL ORDER BY updated_at DESC`, id.WorkspaceID)
	if err != nil {
		problem(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []Design{}
	for rows.Next() {
		var d Design
		d.WorkspaceID = id.WorkspaceID
		if rows.Scan(&d.ID, &d.Name, &d.Document, &d.Version, &d.UpdatedAt) == nil {
			out = append(out, d)
		}
	}
	write(w, 200, out)
}
func (a *API) createDesign(w http.ResponseWriter, r *http.Request) {
	id := identity(r)
	var count int
	_ = a.db.QueryRowContext(r.Context(), `SELECT count(*) FROM designs WHERE workspace_id=$1 AND deleted_at IS NULL`, id.WorkspaceID).Scan(&count)
	if count >= a.maxDesigns {
		problem(w, 429, "workspace design quota reached")
		return
	}
	var d Design
	if decode(w, r, &d) != nil {
		return
	}
	if d.Name == "" {
		d.Name = "Untitled design"
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 500, "save failed")
		return
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(r.Context(), `INSERT INTO designs(workspace_id,owner_id,name,document,current_version) VALUES($1,$2,$3,$4,1) RETURNING id,updated_at`, id.WorkspaceID, id.UserID, d.Name, d.Document).Scan(&d.ID, &d.UpdatedAt)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO design_versions(design_id,version,document,created_by) VALUES($1,1,$2,$3)`, d.ID, d.Document, id.UserID)
	}
	if err != nil {
		problem(w, 500, "save failed")
		return
	}
	_ = tx.Commit()
	d.Version = 1
	d.WorkspaceID = id.WorkspaceID
	a.audit(r, "design.create", d.ID)
	write(w, 201, d)
}
func (a *API) getDesign(w http.ResponseWriter, r *http.Request) {
	id := identity(r)
	var d Design
	d.ID = r.PathValue("id")
	d.WorkspaceID = id.WorkspaceID
	err := a.db.QueryRowContext(r.Context(), `SELECT name,document,current_version,updated_at FROM designs WHERE id=$1 AND workspace_id=$2 AND deleted_at IS NULL`, d.ID, id.WorkspaceID).Scan(&d.Name, &d.Document, &d.Version, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		problem(w, 404, "design not found")
		return
	}
	if err != nil {
		problem(w, 500, "query failed")
		return
	}
	write(w, 200, d)
}
func (a *API) updateDesign(w http.ResponseWriter, r *http.Request) {
	id := identity(r)
	designID := r.PathValue("id")
	var d Design
	if decode(w, r, &d) != nil {
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 500, "save failed")
		return
	}
	defer tx.Rollback()
	var version int
	if d.Version < 1 {
		problem(w, 428, "current design version is required")
		return
	}
	err = tx.QueryRowContext(r.Context(), `UPDATE designs SET name=$1,document=$2,current_version=current_version+1,updated_at=now() WHERE id=$3 AND workspace_id=$4 AND current_version=$5 AND deleted_at IS NULL RETURNING current_version,updated_at`, d.Name, d.Document, designID, id.WorkspaceID, d.Version).Scan(&version, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		var exists bool
		_ = a.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM designs WHERE id=$1 AND workspace_id=$2 AND deleted_at IS NULL)`, designID, id.WorkspaceID).Scan(&exists)
		if exists {
			problem(w, 409, "design was changed in another session")
		} else {
			problem(w, 404, "design not found")
		}
		return
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO design_versions(design_id,version,document,created_by) VALUES($1,$2,$3,$4)`, designID, version, d.Document, id.UserID)
	}
	if err != nil {
		problem(w, 500, "save failed")
		return
	}
	_ = tx.Commit()
	d.ID = designID
	d.Version = version
	d.WorkspaceID = id.WorkspaceID
	a.audit(r, "design.update", designID)
	write(w, 200, d)
}
func (a *API) listVersions(w http.ResponseWriter, r *http.Request) {
	id := identity(r)
	rows, err := a.db.QueryContext(r.Context(), `SELECT v.version,v.document,v.created_at FROM design_versions v JOIN designs d ON d.id=v.design_id WHERE d.id=$1 AND d.workspace_id=$2 ORDER BY v.version DESC LIMIT 50`, r.PathValue("id"), id.WorkspaceID)
	if err != nil {
		problem(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var v int
		var doc json.RawMessage
		var at time.Time
		if rows.Scan(&v, &doc, &at) == nil {
			out = append(out, map[string]any{"version": v, "document": doc, "createdAt": at})
		}
	}
	write(w, 200, out)
}
func (a *API) createShare(w http.ResponseWriter, r *http.Request) {
	id := identity(r)
	token := randomToken()
	expires := time.Now().Add(7 * 24 * time.Hour)
	res, err := a.db.ExecContext(r.Context(), `INSERT INTO share_links(token,design_id,workspace_id,created_by,expires_at) SELECT $1,id,workspace_id,$4,$5 FROM designs WHERE id=$2 AND workspace_id=$3`, token, r.PathValue("id"), id.WorkspaceID, id.UserID, expires)
	if err != nil {
		problem(w, 500, "share failed")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		problem(w, 404, "design not found")
		return
	}
	a.audit(r, "design.share", r.PathValue("id"))
	write(w, 201, map[string]any{"token": token, "expiresAt": expires})
}
func (a *API) getShared(w http.ResponseWriter, r *http.Request) {
	var d Design
	err := a.db.QueryRowContext(r.Context(), `SELECT d.id,d.name,d.document,d.current_version,d.updated_at FROM share_links s JOIN designs d ON d.id=s.design_id WHERE s.token=$1 AND s.revoked_at IS NULL AND s.expires_at>now()`, r.PathValue("token")).Scan(&d.ID, &d.Name, &d.Document, &d.Version, &d.UpdatedAt)
	if err != nil {
		problem(w, 404, "share link unavailable")
		return
	}
	write(w, 200, d)
}
func (a *API) listShares(w http.ResponseWriter, r *http.Request) {
	id := identity(r)
	rows, err := a.db.QueryContext(r.Context(), `SELECT s.token,s.expires_at,s.revoked_at,s.created_at FROM share_links s JOIN designs d ON d.id=s.design_id WHERE d.id=$1 AND d.workspace_id=$2 ORDER BY s.created_at DESC`, r.PathValue("id"), id.WorkspaceID)
	if err != nil {
		problem(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var token string
		var expires, created time.Time
		var revoked sql.NullTime
		if rows.Scan(&token, &expires, &revoked, &created) == nil {
			var revokedAt any = nil
			if revoked.Valid {
				revokedAt = revoked.Time
			}
			out = append(out, map[string]any{"token": token, "expiresAt": expires, "revokedAt": revokedAt, "createdAt": created})
		}
	}
	write(w, 200, out)
}
func (a *API) revokeShare(w http.ResponseWriter, r *http.Request) {
	id := identity(r)
	res, err := a.db.ExecContext(r.Context(), `UPDATE share_links s SET revoked_at=now() FROM designs d WHERE s.token=$1 AND s.design_id=d.id AND d.id=$2 AND d.workspace_id=$3 AND s.revoked_at IS NULL`, r.PathValue("token"), r.PathValue("id"), id.WorkspaceID)
	if err != nil {
		problem(w, 500, "revoke failed")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		problem(w, 404, "active share link not found")
		return
	}
	a.audit(r, "design.share_revoked", r.PathValue("id"))
	w.WriteHeader(204)
}
func (a *API) listAudit(w http.ResponseWriter, r *http.Request) {
	id := identity(r)
	rows, err := a.db.QueryContext(r.Context(), `SELECT action,resource_id,actor_id,created_at FROM audit_events WHERE workspace_id=$1 ORDER BY created_at DESC LIMIT 100`, id.WorkspaceID)
	if err != nil {
		problem(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var action, resource, actor string
		var at time.Time
		if rows.Scan(&action, &resource, &actor, &at) == nil {
			out = append(out, map[string]any{"action": action, "resourceId": resource, "actorId": actor, "createdAt": at})
		}
	}
	write(w, 200, out)
}
func (a *API) audit(r *http.Request, action, resource string) {
	id := identity(r)
	_, _ = a.db.ExecContext(context.Background(), `INSERT INTO audit_events(workspace_id,actor_id,action,resource_id,ip) VALUES($1,$2,$3,$4,$5)`, id.WorkspaceID, id.UserID, action, resource, r.RemoteAddr)
}

func identity(r *http.Request) Identity { return r.Context().Value(identityKey).(Identity) }
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		problem(w, 400, "invalid JSON body")
		return err
	}
	return nil
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func randomToken() string {
	b := make([]byte, 24)
	fillRandom(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, message string) {
	write(w, status, map[string]any{"status": status, "message": message})
}
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := env("WEB_ORIGIN", "http://localhost:3000")
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Dev-Role,X-PrintStudio-Placement")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("method=%s path=%s duration=%s", r.Method, r.URL.Path, time.Since(start))
	})
}

func httpDetect(data []byte) string {
	if len(data) < 12 {
		return "application/octet-stream"
	}
	switch {
	case bytesEqual(data[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}):
		return "image/png"
	case data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return "image/jpeg"
	}
	return "application/octet-stream"
}
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
