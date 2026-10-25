package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/command"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

// Handler serves HTTP endpoints for the understanding parser administration.
type Handler struct {
	templateRepo   domain.SenderTemplateRepository
	extractionRepo domain.ExtractionResultRepository
	publishCmd     *command.PublishTemplateHandler
	metricsCmd     *command.RollupParserMetricsHandler
	sessionMgr     *security.SessionTokenManager
}

// NewHandler creates a new understanding HTTP Handler.
func NewHandler(
	templateRepo domain.SenderTemplateRepository,
	extractionRepo domain.ExtractionResultRepository,
	publishCmd *command.PublishTemplateHandler,
	metricsCmd *command.RollupParserMetricsHandler,
	sessionMgr *security.SessionTokenManager,
) *Handler {
	return &Handler{
		templateRepo:   templateRepo,
		extractionRepo: extractionRepo,
		publishCmd:     publishCmd,
		metricsCmd:     metricsCmd,
		sessionMgr:     sessionMgr,
	}
}

// RegisterRoutes mounts understanding parser admin routes on Chi router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/api/v1/admin/parser", func(r chi.Router) {
		if h.sessionMgr != nil {
			r.Use(h.authMiddleware)
		}
		r.Get("/templates", h.listTemplates)
		r.Post("/templates", h.publishTemplate)
		r.Get("/extractions", h.listExtractions)
		r.Get("/metrics", h.getMetrics)
	})
}

func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) < 7 || authHeader[:7] != "Bearer " {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		token := authHeader[7:]
		claims, err := h.sessionMgr.Validate(token)
		if err != nil || claims == nil {
			http.Error(w, `{"error":"invalid session token"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) listTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := h.templateRepo.ListActive(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var dtos []TemplateDTO
	for _, t := range templates {
		dtos = append(dtos, TemplateDTO{
			ID:            t.ID(),
			SenderPattern: t.SenderPattern(),
			Version:       t.Version(),
			Rules:         t.Rules(),
			Active:        t.Active(),
			CreatedAt:     t.CreatedAt(),
		})
	}

	writeJSON(w, http.StatusOK, dtos)
}

func (h *Handler) listExtractions(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	records, err := h.extractionRepo.ListRecent(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var dtos []ExtractionResultDTO
	for _, rec := range records {
		dtos = append(dtos, ExtractionResultDTO{
			ID:                  rec.ID,
			IngestedMessageID:   rec.IngestedMessageID,
			Method:              rec.Method,
			TemplateVersion:     rec.TemplateVersion,
			Candidate:           &rec.CandidatePayload,
			ConfidenceAggregate: rec.ConfidenceAggregate,
			ConfidencePerField:  rec.ConfidencePerField,
			CreatedAt:           rec.CreatedAt,
		})
	}

	writeJSON(w, http.StatusOK, dtos)
}

func (h *Handler) getMetrics(w http.ResponseWriter, r *http.Request) {
	res, err := h.metricsCmd.Handle(r.Context(), command.RollupParserMetricsCommand{Trigger: "manual"})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var rollups []TemplateRollupDTO
	for _, r := range res.Rollups {
		rollups = append(rollups, TemplateRollupDTO{
			SenderPattern: r.SenderPattern,
			Version:       r.Version,
			Total1h:       r.Total1h,
			Failed1h:      r.Failed1h,
			Rate1h:        r.Rate1h,
			Total24h:      r.Total24h,
			Failed24h:     r.Failed24h,
			Rate24h:       r.Rate24h,
		})
	}

	writeJSON(w, http.StatusOK, ParserMetricsDTO{
		ExaminedTemplates: res.ExaminedTemplates,
		Rollups:           rollups,
		SpikeAlerts:       res.SpikeAlerts,
	})
}

func (h *Handler) publishTemplate(w http.ResponseWriter, r *http.Request) {
	var req PublishTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed request payload"})
		return
	}

	res, err := h.publishCmd.Handle(r.Context(), command.PublishTemplateCommand{
		SenderPattern:   req.SenderPattern,
		Version:         req.Version,
		ExtractionRules: req.ExtractionRules,
	})
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":  err.Error(),
			"result": res,
		})
		return
	}

	writeJSON(w, http.StatusCreated, res)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
