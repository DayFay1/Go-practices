package api

import (
	"encoding/json"
	"net/http"
	"time"

	"polling-system/internal/domain/poll"
	"polling-system/internal/platform/apperr"
)

type createPollRequest struct {
	Title       string   `json:"title"`
	Description *string  `json:"description"`
	StartsAt    *string  `json:"starts_at"`
	EndsAt      *string  `json:"ends_at"`
	Options     []string `json:"options"`
}

type updateStatusRequest struct {
	Status string `json:"status"`
}

type updatePollRequest struct {
	Title       *string        `json:"title"`
	Description optionalString `json:"description,omitempty"`
	StartsAt    optionalString `json:"starts_at,omitempty"`
	EndsAt      optionalString `json:"ends_at,omitempty"`
}

type pollDetailsResponse struct {
	Poll    *poll.Poll    `json:"poll"`
	Options []poll.Option `json:"options"`
}

// @Summary     Create poll
// @Description Admin only
// @Tags        polls
// @Security    BearerAuth
// @Accept      json
// @Produce     json
// @Param       request  body      createPollRequest  true  "Poll payload"
// @Success     201      {object}  map[string]int64
// @Failure     400      {object}  map[string]string  "invalid body"
// @Failure     401      {object}  map[string]string  "unauthorized"
// @Failure     403      {object}  map[string]string  "forbidden"
// @Failure     500      {object}  map[string]string  "server error"
// @Router      /api/v1/polls [post]
func (h *Handler) handleCreatePoll(w http.ResponseWriter, r *http.Request) {
	var req createPollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, apperr.BadRequest("invalid_input", "invalid body", err))
		return
	}

	if req.Title == "" {
		errorResponse(w, apperr.BadRequest("invalid_input", "title is required", nil))
		return
	}

	if len(req.Options) < 2 {
		errorResponse(w, apperr.BadRequest("invalid_input", "at least 2 options are required", nil))
		return
	}

	userID := userIDFromCtx(r)

	startsAt := parseTimePtr(req.StartsAt)
	if req.StartsAt != nil && *req.StartsAt != "" && startsAt == nil {
		errorResponse(w, apperr.BadRequest("invalid_input", "invalid starts_at format", nil))
		return
	}
	endsAt := parseTimePtr(req.EndsAt)
	if req.EndsAt != nil && *req.EndsAt != "" && endsAt == nil {
		errorResponse(w, apperr.BadRequest("invalid_input", "invalid ends_at format", nil))
		return
	}
	if startsAt != nil && endsAt != nil && endsAt.Before(*startsAt) {
		errorResponse(w, apperr.BadRequest("invalid_dates", "ends_at must be after starts_at", nil))
		return
	}

	p := &poll.Poll{
		Title:       req.Title,
		Description: req.Description,
		StartsAt:    startsAt,
		EndsAt:      endsAt,
		CreatorID:   userID,
	}

	opts := make([]poll.Option, 0, len(req.Options))
	for _, text := range req.Options {
		opts = append(opts, poll.Option{Text: text})
	}

	id, err := h.pollSvc.Create(r.Context(), p, opts)
	if err != nil {
		errorResponse(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

// @Summary     List polls
// @Tags        polls
// @Security    BearerAuth
// @Produce     json
// @Param       status  query     string  false  "Filter by status"  Enums(draft,active,closed)
// @Success     200     {array}   poll.Poll
// @Failure     401     {object}  map[string]string  "unauthorized"
// @Failure     500     {object}  map[string]string  "server error"
// @Router      /api/v1/polls [get]
func (h *Handler) handleListPolls(w http.ResponseWriter, r *http.Request) {
	statusParam := r.URL.Query().Get("status")
	var status *string
	if statusParam != "" {
		status = &statusParam
	}
	polls, err := h.pollSvc.List(r.Context(), status)
	if err != nil {
		errorResponse(w, err)
		return
	}
	writeJSON(w, http.StatusOK, polls)
}

// @Summary     Get poll with options
// @Tags        polls
// @Security    BearerAuth
// @Produce     json
// @Param       id   path     int64  true  "Poll ID"
// @Success     200  {object} pollDetailsResponse
// @Failure     400  {object}  map[string]string  "invalid id"
// @Failure     401  {object}  map[string]string  "unauthorized"
// @Failure     404  {object}  map[string]string  "not found"
// @Failure     500  {object}  map[string]string  "server error"
// @Router      /api/v1/polls/{id} [get]
func (h *Handler) handleGetPoll(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		errorResponse(w, apperr.BadRequest("invalid_input", "invalid id", err))
		return
	}

	p, opts, err := h.pollSvc.Get(r.Context(), id)
	if err != nil {
		errorResponse(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"poll":    p,
		"options": opts,
	})
}

// @Summary     Update poll status
// @Description Admin only
// @Tags        polls
// @Security    BearerAuth
// @Accept      json
// @Param       id       path      int64                true  "Poll ID"
// @Param       request  body      updateStatusRequest  true  "New status"
// @Success     204
// @Failure     400      {object}  map[string]string  "invalid id or status"
// @Failure     401      {object}  map[string]string  "unauthorized"
// @Failure     403      {object}  map[string]string  "forbidden"
// @Failure     404      {object}  map[string]string  "not found"
// @Failure     500      {object}  map[string]string  "server error"
// @Router      /api/v1/polls/{id}/status [patch]
func (h *Handler) handleUpdatePollStatus(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		errorResponse(w, apperr.BadRequest("invalid_input", "invalid id", err))
		return
	}

	var req updateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, apperr.BadRequest("invalid_input", "invalid body", err))
		return
	}

	if err := h.pollSvc.UpdateStatus(r.Context(), id, req.Status); err != nil {
		errorResponse(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// @Summary     Update poll (partial)
// @Description Admin only
// @Tags        polls
// @Security    BearerAuth
// @Accept      json
// @Param       id       path      int64               true  "Poll ID"
// @Param       request  body      updatePollRequest   true  "Poll fields"
// @Success     204
// @Failure     400      {object}  map[string]string  "invalid input"
// @Failure     401      {object}  map[string]string  "unauthorized"
// @Failure     403      {object}  map[string]string  "forbidden"
// @Failure     404      {object}  map[string]string  "not found"
// @Failure     500      {object}  map[string]string  "server error"
// @Router      /api/v1/polls/{id} [patch]
func (h *Handler) handleUpdatePoll(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		errorResponse(w, apperr.BadRequest("invalid_input", "invalid id", err))
		return
	}

	var req updatePollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, apperr.BadRequest("invalid_input", "invalid body", err))
		return
	}

	if req.Title != nil && *req.Title == "" {
		errorResponse(w, apperr.BadRequest("invalid_input", "title is required", nil))
		return
	}

	var startsAt poll.OptionalTime
	startsAt.Set = req.StartsAt.Set
	if req.StartsAt.Set {
		if req.StartsAt.Value == nil || *req.StartsAt.Value == "" {
			startsAt.Value = nil
		} else {
			t, err := time.Parse(time.RFC3339, *req.StartsAt.Value)
			if err != nil {
				errorResponse(w, apperr.BadRequest("invalid_input", "invalid starts_at format", err))
				return
			}
			startsAt.Value = &t
		}
	}

	var endsAt poll.OptionalTime
	endsAt.Set = req.EndsAt.Set
	if req.EndsAt.Set {
		if req.EndsAt.Value == nil || *req.EndsAt.Value == "" {
			endsAt.Value = nil
		} else {
			t, err := time.Parse(time.RFC3339, *req.EndsAt.Value)
			if err != nil {
				errorResponse(w, apperr.BadRequest("invalid_input", "invalid ends_at format", err))
				return
			}
			endsAt.Value = &t
		}
	}

	if req.Title == nil && !req.Description.Set && !startsAt.Set && !endsAt.Set {
		errorResponse(w, apperr.BadRequest("invalid_input", "no fields to update", nil))
		return
	}

	input := poll.UpdateInput{
		Title:       req.Title,
		Description: poll.OptionalString{Set: req.Description.Set, Value: req.Description.Value},
		StartsAt:    startsAt,
		EndsAt:      endsAt,
	}

	if err := h.pollSvc.Update(r.Context(), id, input); err != nil {
		errorResponse(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// @Summary     Delete poll
// @Description Admin only
// @Tags        polls
// @Security    BearerAuth
// @Param       id   path  int64  true  "Poll ID"
// @Success     204
// @Failure     401  {object}  map[string]string  "unauthorized"
// @Failure     403  {object}  map[string]string  "forbidden"
// @Failure     404  {object}  map[string]string  "not found"
// @Failure     500  {object}  map[string]string  "server error"
// @Router      /api/v1/polls/{id} [delete]
func (h *Handler) handleDeletePoll(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		errorResponse(w, apperr.BadRequest("invalid_input", "invalid id", err))
		return
	}

	if err := h.pollSvc.Delete(r.Context(), id); err != nil {
		errorResponse(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
