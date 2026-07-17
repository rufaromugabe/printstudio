"use client";

import { FormEvent, useEffect, useState } from "react";
import { api, AuditEvent, ICCProfile, Product, ProductionMetrics } from "@/lib/api";

type AdminTab = "products" | "members" | "audit" | "metrics" | "icc";
type Membership = { userId: string; email: string; displayName: string; role: string; kind: string; createdAt?: string };

const METHOD_OPTIONS = ["DTF", "Screen print", "Vinyl", "Embroidery", "Sublimation"];
const COMMON_ICC_IDS = [
  { id: "srgb", label: "sRGB" },
  { id: "display-p3", label: "Display P3" },
  { id: "gray-gamma-22", label: "Gray Gamma 2.2" },
];

const emptyTemplate = `{
  "version": 1,
  "category": "apparel",
  "views": [
    {
      "id": "front",
      "label": "Front",
      "canvasWidth": 420,
      "canvasHeight": 460,
      "physicalWidthMm": 300,
      "physicalHeightMm": 400,
      "safeMarginMm": 8,
      "bleedMm": 3,
      "mockup": { "kind": "shirt" }
    }
  ],
  "properties": [],
  "colors": [
    { "value": "#f4f1e9", "label": "Natural" },
    { "value": "#17191c", "label": "Black" }
  ]
}`;

function viewsFromTemplate(templateJson: string): string[] {
  try {
    const parsed = JSON.parse(templateJson) as { views?: { id?: string }[] };
    return (parsed.views ?? []).map((view) => view.id).filter((id): id is string => Boolean(id));
  } catch {
    return [];
  }
}

export function AdminPanel({
  products,
  onProductsChange,
}: {
  products: Product[];
  onProductsChange: (products: Product[]) => void;
}) {
  const [tab, setTab] = useState<AdminTab>("products");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [members, setMembers] = useState<Membership[]>([]);
  const [membersState, setMembersState] = useState<"idle" | "loading" | "ready" | "error">("idle");
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteRole, setInviteRole] = useState("member");

  const [productId, setProductId] = useState("");
  const [productName, setProductName] = useState("");
  const [methods, setMethods] = useState<string[]>(["DTF"]);
  const [active, setActive] = useState(true);
  const [templateJson, setTemplateJson] = useState(emptyTemplate);

  const [audit, setAudit] = useState<AuditEvent[]>([]);
  const [auditState, setAuditState] = useState<"idle" | "loading" | "ready" | "error">("idle");
  const [metrics, setMetrics] = useState<ProductionMetrics | null>(null);
  const [metricsState, setMetricsState] = useState<"idle" | "loading" | "ready" | "error">("idle");
  const [profiles, setProfiles] = useState<ICCProfile[]>([]);
  const [iccState, setIccState] = useState<"idle" | "loading" | "ready" | "error" | "unavailable">("idle");
  const [iccId, setIccId] = useState("srgb");
  const [iccLabel, setIccLabel] = useState("sRGB");
  const [iccFile, setIccFile] = useState<File | null>(null);

  const loadProduct = (product: Product | null) => {
    if (!product) {
      setProductId("");
      setProductName("");
      setMethods(["DTF"]);
      setActive(true);
      setTemplateJson(emptyTemplate);
      return;
    }
    setProductId(product.id);
    setProductName(product.name);
    setMethods(product.methods?.length ? product.methods : ["DTF"]);
    setActive(product.active);
    setTemplateJson(JSON.stringify(product.template, null, 2));
  };

  const loadAudit = async () => {
    setAuditState("loading");
    setError("");
    try {
      setAudit(await api.audit());
      setAuditState("ready");
    } catch (err) {
      setAuditState("error");
      setError(err instanceof Error ? err.message : "Could not load audit log");
    }
  };

  const loadMetrics = async () => {
    setMetricsState("loading");
    setError("");
    try {
      setMetrics(await api.productionMetrics());
      setMetricsState("ready");
    } catch (err) {
      setMetricsState("error");
      setError(err instanceof Error ? err.message : "Could not load metrics");
    }
  };

  const loadMembers = async () => {
    setMembersState("loading");
    setError("");
    try {
      setMembers(await api.listMemberships());
      setMembersState("ready");
    } catch (err) {
      setMembersState("error");
      setError(err instanceof Error ? err.message : "Could not load members");
    }
  };

  const loadProfiles = async () => {
    setIccState("loading");
    setError("");
    try {
      const result = await api.iccProfiles();
      setProfiles(result.profiles ?? []);
      setIccState("ready");
    } catch (err) {
      const text = err instanceof Error ? err.message : "Could not load ICC profiles";
      if (text.toLowerCase().includes("not configured") || text.includes("501")) {
        setIccState("unavailable");
      } else {
        setIccState("error");
      }
      setError(text);
    }
  };

  useEffect(() => {
    if (tab === "members" && membersState === "idle") void loadMembers();
    if (tab === "audit" && auditState === "idle") void loadAudit();
    if (tab === "metrics" && metricsState === "idle") void loadMetrics();
    if (tab === "icc" && iccState === "idle") void loadProfiles();
  }, [tab, membersState, auditState, metricsState, iccState]);

  const saveProduct = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setMessage("");
    setError("");
    try {
      let template: Product["template"];
      try {
        template = JSON.parse(templateJson) as Product["template"];
      } catch {
        throw new Error("Product template must be valid JSON");
      }
      const views = viewsFromTemplate(templateJson);
      if (!productId.trim() || !productName.trim()) throw new Error("Product id and name are required");
      if (!methods.length) throw new Error("Select at least one decoration method");
      if (!views.length) throw new Error("Template must include at least one view with an id");
      const saved = await api.upsertProduct({
        id: productId.trim(),
        name: productName.trim(),
        methods,
        views,
        active,
        template,
      });
      const next = [...products.filter((item) => item.id !== saved.id), saved].sort((a, b) => a.name.localeCompare(b.name));
      onProductsChange(next);
      loadProduct(saved);
      setMessage(`Saved ${saved.name}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not save product");
    } finally {
      setBusy(false);
    }
  };

  const toggleMethod = (method: string) => {
    setMethods((current) => (current.includes(method) ? current.filter((item) => item !== method) : [...current, method]));
  };

  const uploadProfile = async (event: FormEvent) => {
    event.preventDefault();
    if (!iccFile) {
      setError("Choose an .icc file to upload");
      return;
    }
    setBusy(true);
    setMessage("");
    setError("");
    try {
      const saved = await api.uploadIccProfile(iccFile, { id: iccId, label: iccLabel || COMMON_ICC_IDS.find((item) => item.id === iccId)?.label });
      setProfiles((items) => {
        const next = items.filter((item) => item.id !== saved.id);
        return [...next, saved].sort((a, b) => a.id.localeCompare(b.id));
      });
      setIccState("ready");
      setIccFile(null);
      setMessage(`Updated ${saved.label} (v${saved.version})`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "ICC upload failed");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="admin-panel">
      <div className="panel-title">
        <p className="eyebrow">ADMIN</p>
        <h2>Workspace controls</h2>
        <p>Manage catalog products, members, audit activity, production metrics, and common ICC profiles — separate from the design editor.</p>
      </div>

      <div className="admin-tabs" role="tablist" aria-label="Admin sections">
        {([
          ["products", "Products"],
          ["members", "Members"],
          ["audit", "Audit"],
          ["metrics", "Metrics"],
          ["icc", "ICC"],
        ] as const).map(([id, label]) => (
          <button key={id} type="button" role="tab" aria-selected={tab === id} className={tab === id ? "active" : ""} onClick={() => { setTab(id); setMessage(""); setError(""); }}>
            {label}
          </button>
        ))}
      </div>

      {(message || error) && (
        <p className={error ? "admin-status error" : "admin-status"} role="status">
          {error || message}
        </p>
      )}

      {tab === "products" && (
        <div className="admin-section">
          <div className="admin-layout">
            <div>
              <div className="panel-section-head">
                <strong>Catalog</strong>
                <span>{products.length}</span>
              </div>
              <div className="admin-product-list">
                <button type="button" className={!productId ? "active" : ""} onClick={() => loadProduct(null)}>
                  + New product
                </button>
                {products.map((product) => (
                  <button key={product.id} type="button" className={productId === product.id ? "active" : ""} onClick={() => loadProduct(product)}>
                    <strong>{product.name}</strong>
                    <small>{product.id} · {product.active ? "active" : "inactive"}</small>
                  </button>
                ))}
              </div>
            </div>
            <form className="admin-form" onSubmit={(event) => void saveProduct(event)}>
              <label>
                Product id
                <input value={productId} onChange={(event) => setProductId(event.target.value)} placeholder="classic-tee" required pattern="[a-z][a-z0-9_-]{0,63}" />
              </label>
              <label>
                Name
                <input value={productName} onChange={(event) => setProductName(event.target.value)} placeholder="Classic Tee" required />
              </label>
              <fieldset>
                <legend>Methods</legend>
                <div className="admin-chip-row">
                  {METHOD_OPTIONS.map((method) => (
                    <label key={method} className="admin-chip">
                      <input type="checkbox" checked={methods.includes(method)} onChange={() => toggleMethod(method)} />
                      {method}
                    </label>
                  ))}
                </div>
              </fieldset>
              <label className="admin-check">
                <input type="checkbox" checked={active} onChange={(event) => setActive(event.target.checked)} />
                Active in studio catalog
              </label>
              <label>
                Template JSON
                <textarea className="admin-template" value={templateJson} onChange={(event) => setTemplateJson(event.target.value)} spellCheck={false} />
              </label>
              <button className="button primary panel-action" type="submit" disabled={busy}>
                {busy ? "Saving…" : "Save product"}
              </button>
            </form>
          </div>
        </div>
      )}

      {tab === "members" && (
        <div className="admin-section">
          <div className="panel-section-head">
            <strong>Members & invites</strong>
            <button type="button" className="admin-refresh" onClick={() => void loadMembers()} disabled={membersState === "loading"}>
              Refresh
            </button>
          </div>
          <form className="admin-invite" onSubmit={(event) => {
            event.preventDefault();
            void (async () => {
              setBusy(true); setMessage(""); setError("");
              try {
                await api.inviteMembership(inviteEmail.trim(), inviteRole);
                setInviteEmail("");
                setMessage(`Invited ${inviteEmail.trim()} as ${inviteRole}`);
                await loadMembers();
              } catch (err) {
                setError(err instanceof Error ? err.message : "Invite failed");
              } finally {
                setBusy(false);
              }
            })();
          }}>
            <label>Email<input type="email" required value={inviteEmail} onChange={(e) => setInviteEmail(e.target.value)} placeholder="teammate@studio.com" /></label>
            <label>Role<select value={inviteRole} onChange={(e) => setInviteRole(e.target.value)}><option value="admin">Admin</option><option value="member">Member</option><option value="viewer">Viewer</option></select></label>
            <button className="button primary" type="submit" disabled={busy}>{busy ? "Saving…" : "Invite"}</button>
          </form>
          <p className="admin-note">Existing users are added immediately. New emails become pending invites and join this workspace on next Google sign-in. Viewers can browse but not save, upload, or export.</p>
          {membersState === "loading" && <div className="panel-empty">Loading members…</div>}
          {membersState === "error" && <div className="panel-empty"><strong>Members unavailable</strong><span>{error || "Try again shortly."}</span></div>}
          {membersState === "ready" && (
            <div className="admin-members">
              {members.map((row) => (
                <div className="admin-member-row" key={`${row.kind}-${row.userId}`}>
                  <div>
                    <strong>{row.displayName || row.email}</strong>
                    <small>{row.email} · {row.kind === "invite" ? "Pending invite" : row.role}</small>
                  </div>
                  {row.role === "owner" ? <span>Owner</span> : (
                    <select value={row.role} disabled={row.kind === "invite" || busy} onChange={(e) => {
                      void (async () => {
                        setBusy(true); setError("");
                        try {
                          await api.updateMembership(row.userId, e.target.value);
                          await loadMembers();
                          setMessage(`Updated ${row.email} to ${e.target.value}`);
                        } catch (err) {
                          setError(err instanceof Error ? err.message : "Role update failed");
                        } finally {
                          setBusy(false);
                        }
                      })();
                    }}>
                      <option value="admin">Admin</option>
                      <option value="member">Member</option>
                      <option value="viewer">Viewer</option>
                    </select>
                  )}
                  {row.role === "owner" ? <span /> : (
                    <button type="button" className="button secondary danger" disabled={busy} onClick={() => {
                      void (async () => {
                        setBusy(true); setError("");
                        try {
                          await api.removeMembership(row.userId);
                          await loadMembers();
                          setMessage(`Removed ${row.email}`);
                        } catch (err) {
                          setError(err instanceof Error ? err.message : "Remove failed");
                        } finally {
                          setBusy(false);
                        }
                      })();
                    }}>Remove</button>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {tab === "audit" && (
        <div className="admin-section">
          <div className="panel-section-head">
            <strong>Recent events</strong>
            <button type="button" className="admin-refresh" onClick={() => void loadAudit()} disabled={auditState === "loading"}>
              Refresh
            </button>
          </div>
          {auditState === "loading" && <div className="panel-empty">Loading audit log…</div>}
          {auditState === "error" && <div className="panel-empty"><strong>Audit unavailable</strong><span>{error || "Try again shortly."}</span></div>}
          {auditState === "ready" && audit.length === 0 && <div className="panel-empty"><strong>No events yet</strong><span>Workspace actions will appear here.</span></div>}
          {auditState === "ready" && audit.length > 0 && (
            <div className="admin-list">
              {audit.map((event, index) => (
                <article key={`${event.createdAt}-${event.action}-${index}`}>
                  <strong>{event.action}</strong>
                  <small>{new Date(event.createdAt).toLocaleString()}</small>
                  <span>Resource {event.resourceId || "—"}</span>
                  <span>Actor {event.actorId || "—"}</span>
                </article>
              ))}
            </div>
          )}
        </div>
      )}

      {tab === "metrics" && (
        <div className="admin-section">
          <div className="panel-section-head">
            <strong>Production counters</strong>
            <button type="button" className="admin-refresh" onClick={() => void loadMetrics()} disabled={metricsState === "loading"}>
              Refresh
            </button>
          </div>
          {metricsState === "loading" && <div className="panel-empty">Loading metrics…</div>}
          {metricsState === "error" && <div className="panel-empty"><strong>Metrics unavailable</strong><span>{error || "Try again shortly."}</span></div>}
          {metrics && metricsState === "ready" && (
            <>
              <div className="admin-metric-grid">
                {Object.entries(metrics.counters).map(([key, value]) => (
                  <div key={key}>
                    <strong>{value}</strong>
                    <span>{key}</span>
                  </div>
                ))}
              </div>
              <div className="panel-section-head"><strong>Runtime</strong></div>
              <div className="admin-kv">
                <div><span>Require natives</span><strong>{metrics.requireNatives ? "yes" : "no"}</strong></div>
                <div><span>Require ICC</span><strong>{metrics.requireIcc ? "yes" : "no"}</strong></div>
                <div><span>ICC profiles</span><strong>{metrics.capabilities.iccProfiles ? "ready" : "missing"}</strong></div>
                <div><span>Max render px</span><strong>{metrics.capabilities.maxRenderPixels ?? "—"}</strong></div>
                <div><span>VIPS</span><strong>{metrics.capabilities.vipsPath || "not set"}</strong></div>
                <div><span>Potrace</span><strong>{metrics.capabilities.potracePath || "not set"}</strong></div>
              </div>
            </>
          )}
        </div>
      )}

      {tab === "icc" && (
        <div className="admin-section">
          <div className="panel-section-head">
            <strong>Common profiles</strong>
            <button type="button" className="admin-refresh" onClick={() => void loadProfiles()} disabled={iccState === "loading"}>
              Refresh
            </button>
          </div>
          {iccState === "loading" && <div className="panel-empty">Loading ICC profiles…</div>}
          {iccState === "unavailable" && (
            <div className="panel-empty">
              <strong>ICC store not configured</strong>
              <span>Set ICC_PROFILE_DIR on the API to enable profile listing and refresh uploads.</span>
            </div>
          )}
          {iccState === "error" && <div className="panel-empty"><strong>Could not load profiles</strong><span>{error}</span></div>}
          {iccState === "ready" && (
            <div className="admin-list">
              {profiles.length === 0 ? (
                <div className="panel-empty"><strong>No profiles on disk</strong><span>Upload a common profile below.</span></div>
              ) : (
                profiles.map((profile) => (
                  <article key={profile.id}>
                    <strong>{profile.label || profile.id}</strong>
                    <small>v{profile.version} · {(profile.size / 1024).toFixed(1)} KB</small>
                    <span>{profile.id}</span>
                    <span>{profile.sha256.slice(0, 12)}…</span>
                  </article>
                ))
              )}
            </div>
          )}

          {(iccState === "ready" || iccState === "unavailable") && (
            <form className="admin-form" onSubmit={(event) => void uploadProfile(event)}>
              <p className="admin-note">Only common bundled ids can be refreshed: srgb, display-p3, gray-gamma-22.</p>
              <label>
                Profile id
                <select
                  value={iccId}
                  onChange={(event) => {
                    const next = event.target.value;
                    setIccId(next);
                    setIccLabel(COMMON_ICC_IDS.find((item) => item.id === next)?.label ?? next);
                  }}
                >
                  {COMMON_ICC_IDS.map((item) => (
                    <option key={item.id} value={item.id}>{item.label}</option>
                  ))}
                </select>
              </label>
              <label>
                Label
                <input value={iccLabel} onChange={(event) => setIccLabel(event.target.value)} />
              </label>
              <label>
                ICC file
                <input
                  type="file"
                  accept=".icc,.icm,application/vnd.iccprofile,application/octet-stream"
                  onChange={(event) => setIccFile(event.target.files?.[0] ?? null)}
                />
              </label>
              <button className="button primary panel-action" type="submit" disabled={busy || !iccFile || iccState === "unavailable"}>
                {busy ? "Uploading…" : "Upload profile"}
              </button>
            </form>
          )}
        </div>
      )}
    </div>
  );
}
