package assets

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/audit"
	"github.com/3lbits/vigil/internal/authz"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/httputil"
	"github.com/3lbits/vigil/internal/locale"
	"github.com/3lbits/vigil/internal/middleware"
	assetstemplates "github.com/3lbits/vigil/internal/modules/assets/templates"
	"github.com/3lbits/vigil/internal/ui/layout"
)

const assetsPageSize int32 = 50

type Handler struct {
	q      db.Querier
	engine *authz.Engine
}

func NewHandler(q db.Querier, engine *authz.Engine) *Handler {
	return &Handler{q: q, engine: engine}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	qv := r.URL.Query()
	search := strings.TrimSpace(qv.Get("q"))
	status := strings.TrimSpace(qv.Get("status"))
	flash := qv.Get("flash")
	flashType := qv.Get("type")

	var offset int32
	if n, err := strconv.ParseInt(qv.Get("offset"), 10, 32); err == nil && n > 0 {
		offset = int32(n)
	}

	rows, err := h.q.ListAssets(r.Context(), db.ListAssetsParams{
		Q:          search,
		Status:     status,
		PageSize:   assetsPageSize + 1,
		PageOffset: offset,
	})
	if err != nil {
		slog.Error("list assets", "error", err)
		http.Error(w, "failed to load assets", http.StatusInternalServerError)
		return
	}

	hasMore := len(rows) > int(assetsPageSize)
	if hasMore {
		rows = rows[:assetsPageSize]
	}

	user, _ := middleware.FromContext(r.Context())
	if r.Header.Get("HX-Request") == "true" {
		httputil.Render(w, r, assetstemplates.AssetTable(rows, hasMore, offset+assetsPageSize, search, status))
		return
	}
	httputil.Render(w, r, layout.Layout("page_assets_title", "page_assets_subtitle", "assets", user,
		assetstemplates.AssetList(rows, search, status, flash, flashType, hasMore, offset+assetsPageSize),
	))
}

func (h *Handler) New(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.FromContext(r.Context())
	httputil.Render(w, r, layout.Layout("page_assets_add_title", "page_assets_add_subtitle", "assets", user,
		assetstemplates.AssetForm(nil, "", "", nil),
	))
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if parseErr := r.ParseForm(); parseErr != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	status := validAssetStatus(r.FormValue("status"))
	criticality := validAssetCriticality(r.FormValue("criticality"))
	created, err := h.q.CreateAsset(r.Context(), db.CreateAssetParams{
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: strings.TrimSpace(r.FormValue("description")),
		AssetType:   strings.TrimSpace(r.FormValue("asset_type")),
		Owner:       strings.TrimSpace(r.FormValue("owner")),
		Status:      status,
		Criticality: criticality,
	})
	if err != nil {
		slog.Error("create asset", "error", err)
		user, _ := middleware.FromContext(r.Context())
		httputil.Render(w, r, layout.Layout("page_assets_add_title", "page_assets_add_subtitle", "assets", user,
			assetstemplates.AssetForm(nil, locale.T(r.Context(), "assets_flash_create_failed"), "error", nil),
		))
		return
	}

	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "assets.asset.create",
		Attrs: map[string]any{"asset_id": created.ID.String(), "name": created.Name},
	})
	httputil.RedirectWithFlash(w, r, "/assets", locale.T(r.Context(), "assets_flash_created"), "success")
}

func (h *Handler) Show(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	asset, err := h.q.GetAsset(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	user, _ := middleware.FromContext(r.Context())
	canEdit, err := h.engine.Allow(r.Context(), user.ID, user.Role, "assets", "write")
	if err != nil {
		slog.Error("authz eval assets", "error", err)
	}
	flash := r.URL.Query().Get("flash")
	flashType := r.URL.Query().Get("type")
	auditLog, err := h.q.ListAuditLogForAsset(r.Context(), id.String())
	if err != nil {
		slog.Warn("list audit log for asset", "asset_id", id.String(), "error", err) // #nosec G706
	}
	httputil.Render(w, r, layout.Layout(asset.Name, "page_assets_detail_subtitle", "assets", user,
		assetstemplates.AssetDetail(asset, canEdit, flash, flashType, auditLog),
	))
}

func (h *Handler) Edit(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	asset, err := h.q.GetAsset(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	auditLog, err := h.q.ListAuditLogForAsset(r.Context(), id.String())
	if err != nil {
		slog.Warn("list audit log for asset edit", "asset_id", id.String(), "error", err) // #nosec G706
	}
	user, _ := middleware.FromContext(r.Context())
	httputil.Render(w, r, layout.Layout("page_assets_edit_title", asset.Name, "assets", user,
		assetstemplates.AssetForm(&asset, "", "", auditLog),
	))
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if parseErr := r.ParseForm(); parseErr != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	status := validAssetStatus(r.FormValue("status"))
	criticality := validAssetCriticality(r.FormValue("criticality"))
	updated, err := h.q.UpdateAsset(r.Context(), db.UpdateAssetParams{
		ID:          id,
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: strings.TrimSpace(r.FormValue("description")),
		AssetType:   strings.TrimSpace(r.FormValue("asset_type")),
		Owner:       strings.TrimSpace(r.FormValue("owner")),
		Status:      status,
		Criticality: criticality,
	})
	if err != nil {
		slog.Error("update asset", "error", err)
		user, _ := middleware.FromContext(r.Context())
		fallback := db.Asset{ID: id, Name: strings.TrimSpace(r.FormValue("name"))}
		httputil.Render(w, r, layout.Layout("page_assets_edit_title", "page_assets_item_subtitle", "assets", user,
			assetstemplates.AssetForm(&fallback, locale.T(r.Context(), "assets_flash_update_failed"), "error", nil),
		))
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "assets.asset.update",
		Attrs: map[string]any{"asset_id": updated.ID.String(), "name": updated.Name},
	})
	httputil.RedirectWithFlash(w, r, "/assets/"+updated.ID.String(), locale.T(r.Context(), "assets_flash_updated"), "success")
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.q.DeleteAsset(r.Context(), id); err != nil {
		slog.Error("delete asset", "error", err)
		httputil.RedirectWithFlash(w, r, "/assets", locale.T(r.Context(), "assets_flash_delete_failed"), "error")
		return
	}
	audit.RecordOrWarn(r.Context(), h.q, audit.Event{
		Event: "assets.asset.delete",
		Attrs: map[string]any{"asset_id": id.String()},
	})
	httputil.RedirectWithFlash(w, r, "/assets", locale.T(r.Context(), "assets_flash_deleted"), "success")
}

func validAssetStatus(v string) string {
	switch v {
	case "planned", "retired":
		return v
	default:
		return "active"
	}
}

func validAssetCriticality(v string) string {
	switch v {
	case "low", "high":
		return v
	default:
		return "medium"
	}
}
