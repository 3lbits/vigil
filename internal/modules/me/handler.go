package me

import (
	"log/slog"
	"net/http"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/httputil"
	"github.com/3lbits/vigil/internal/middleware"
	metemplates "github.com/3lbits/vigil/internal/modules/me/templates"
	"github.com/3lbits/vigil/internal/ui/layout"
	"github.com/google/uuid"
)

type Handler struct {
	q db.Querier
}

func NewHandler(q db.Querier) *Handler {
	return &Handler{q: q}
}

func (h *Handler) Show(w http.ResponseWriter, r *http.Request) {
	sessionUser, ok := middleware.FromContext(r.Context())
	if !ok {
		http.Error(w, "missing user context", http.StatusUnauthorized)
		return
	}

	vm := metemplates.PageVM{
		Profile: metemplates.ProfileVM{
			Name:     sessionUser.Name,
			Email:    sessionUser.Email,
			Role:     sessionUser.Role,
			Provider: "-",
			OrgName:  "-",
		},
	}

	userID, err := uuid.Parse(sessionUser.ID)
	if err != nil {
		slog.Warn("parse user id for my page", "user_id", sessionUser.ID, "error", err)
		httputil.Render(w, r, layout.Layout("page_my_title", "page_my_subtitle", "me", sessionUser, metemplates.Page(vm)))
		return
	}

	dbUser, err := h.q.GetUserByID(r.Context(), userID)
	if err == nil {
		vm.Profile.Provider = dbUser.Provider
		if dbUser.OrgID.Valid {
			org, orgErr := h.q.GetOrganization(r.Context(), dbUser.OrgID.UUID)
			if orgErr != nil {
				slog.Warn("load organization for my page", "user_id", userID, "org_id", dbUser.OrgID.UUID, "error", orgErr)
			} else {
				vm.Profile.OrgName = org.Name
			}
		}
	}

	uid := uuid.NullUUID{UUID: userID, Valid: true}

	activities, err := h.q.ListOwnedActivities(r.Context(), uid)
	if err != nil {
		slog.Error("list owned activities", "user_id", userID, "error", err)
		http.Error(w, "failed to load page", http.StatusInternalServerError)
		return
	}
	vm.Activities = activities

	measures, err := h.q.ListOwnedMeasures(r.Context(), uid)
	if err != nil {
		slog.Error("list owned measures", "user_id", userID, "error", err)
		http.Error(w, "failed to load page", http.StatusInternalServerError)
		return
	}
	vm.Measures = measures

	risks, err := h.q.ListOwnedRisks(r.Context(), uid)
	if err != nil {
		slog.Error("list owned risks", "user_id", userID, "error", err)
		http.Error(w, "failed to load page", http.StatusInternalServerError)
		return
	}
	vm.Risks = risks

	httputil.Render(w, r, layout.Layout("page_my_title", "page_my_subtitle", "me", sessionUser, metemplates.Page(vm)))
}
