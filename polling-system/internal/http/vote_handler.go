package api

import (
	"encoding/json"
	"net/http"

	"polling-system/internal/domain/vote"
	"polling-system/internal/platform/apperr"
	"polling-system/internal/worker"
)

type voteRequest struct {
	OptionID int64 `json:"option_id"`
}

type pollResultsResponse struct {
	PollID     int64         `json:"poll_id"`
	TotalVotes int64         `json:"total_votes"`
	Options    []vote.Result `json:"options"`
}

// @Summary     Vote for an option
// @Tags        votes
// @Security    BearerAuth
// @Accept      json
// @Param       id       path      int64        true  "Poll ID"
// @Param       request  body      voteRequest  true  "Vote payload"
// @Success     204
// @Failure     400      {object}  map[string]string  "invalid body or already voted"
// @Failure     401      {object}  map[string]string  "unauthorized"
// @Failure     404      {object}  map[string]string  "not found"
// @Failure     409      {object}  map[string]string  "already voted"
// @Failure     500      {object}  map[string]string  "server error"
// @Router      /api/v1/polls/{id}/vote [post]
func (h *Handler) handleVote(w http.ResponseWriter, r *http.Request) {
	pollID, err := parseIDParam(r, "id")
	if err != nil {
		errorResponse(w, apperr.BadRequest("invalid_input", "invalid poll id", err))
		return
	}

	var req voteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, apperr.BadRequest("invalid_input", "invalid body", err))
		return
	}
	if req.OptionID == 0 {
		errorResponse(w, apperr.BadRequest("invalid_input", "option_id is required", nil))
		return
	}

	userID := userIDFromCtx(r)

	if err := h.voteSvc.Vote(r.Context(), pollID, req.OptionID, userID); err != nil {
		errorResponse(w, err)
		return
	}

	select {
	case h.voteCh <- worker.VoteEvent{PollID: pollID, OptionID: req.OptionID, UserID: userID}:
	default:
		if h.agg == nil {
			slogLogger.Warn("vote event queue full; event dropped", "poll_id", pollID, "option_id", req.OptionID, "user_id", userID)
			break
		}
		if err := h.agg.IncrementAggregated(r.Context(), pollID, req.OptionID); err != nil {
			slogLogger.Warn("vote event queue full; failed to aggregate synchronously", "poll_id", pollID, "option_id", req.OptionID, "user_id", userID, "error", err)
			break
		}
		slogLogger.Warn("vote event queue full; aggregated synchronously", "poll_id", pollID, "option_id", req.OptionID, "user_id", userID)
	}

	w.WriteHeader(http.StatusNoContent)
}

// @Summary     Poll results
// @Tags        polls
// @Security    BearerAuth
// @Produce     json
// @Param       id   path     int64  true  "Poll ID"
// @Success     200  {object} pollResultsResponse
// @Failure     400  {object}  map[string]string  "invalid poll id"
// @Failure     401  {object}  map[string]string  "unauthorized"
// @Failure     404  {object}  map[string]string  "not found"
// @Failure     500  {object}  map[string]string  "server error"
// @Router      /api/v1/polls/{id}/results [get]
func (h *Handler) handlePollResults(w http.ResponseWriter, r *http.Request) {
	pollID, err := parseIDParam(r, "id")
	if err != nil {
		errorResponse(w, apperr.BadRequest("invalid_input", "invalid poll id", err))
		return
	}

	res, total, err := h.voteSvc.Results(r.Context(), pollID)
	if err != nil {
		errorResponse(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"poll_id":     pollID,
		"total_votes": total,
		"options":     res,
	})
}
