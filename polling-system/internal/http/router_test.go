package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"polling-system/internal/domain/poll"
	"polling-system/internal/domain/user"
	"polling-system/internal/domain/vote"
	jwtpkg "polling-system/internal/platform/jwt"
	"polling-system/internal/worker"
)

type testUserRepo struct {
	mu     sync.Mutex
	users  map[int64]*user.User
	byMail map[string]int64
	nextID int64
}

func newTestUserRepo() *testUserRepo {
	return &testUserRepo{
		users:  make(map[int64]*user.User),
		byMail: make(map[string]int64),
		nextID: 1,
	}
}

func (r *testUserRepo) seed(u *user.User) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u.ID == 0 {
		u.ID = r.nextID
		r.nextID++
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now()
	}
	if !u.IsActive {
		u.IsActive = true
	}
	copyUser := *u
	r.users[u.ID] = &copyUser
	r.byMail[u.Email] = u.ID
}

func (r *testUserRepo) Create(ctx context.Context, u *user.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u.ID = r.nextID
	r.nextID++
	u.CreatedAt = time.Now()
	if !u.IsActive {
		u.IsActive = true
	}
	copyUser := *u
	r.users[u.ID] = &copyUser
	r.byMail[u.Email] = u.ID
	return nil
}

func (r *testUserRepo) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byMail[email]
	if !ok {
		return nil, sql.ErrNoRows
	}
	copyUser := *r.users[id]
	return &copyUser, nil
}

func (r *testUserRepo) GetByID(ctx context.Context, id int64) (*user.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	copyUser := *u
	return &copyUser, nil
}

func (r *testUserRepo) List(ctx context.Context) ([]user.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	res := make([]user.User, 0, len(r.users))
	for _, u := range r.users {
		res = append(res, *u)
	}
	return res, nil
}

func (r *testUserRepo) UpdateRole(ctx context.Context, id int64, role string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return sql.ErrNoRows
	}
	u.Role = role
	return nil
}

func (r *testUserRepo) Deactivate(ctx context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return sql.ErrNoRows
	}
	u.IsActive = false
	return nil
}

type testPollRepo struct {
	mu           sync.Mutex
	polls        map[int64]*poll.Poll
	opts         map[int64][]poll.Option
	nextPollID   int64
	nextOptionID int64
}

func newTestPollRepo() *testPollRepo {
	return &testPollRepo{
		polls:        make(map[int64]*poll.Poll),
		opts:         make(map[int64][]poll.Option),
		nextPollID:   1,
		nextOptionID: 1,
	}
}

func (r *testPollRepo) Create(ctx context.Context, p *poll.Poll, options []poll.Option) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p.ID = r.nextPollID
	r.nextPollID++
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	copyPoll := *p
	r.polls[p.ID] = &copyPoll

	cloned := make([]poll.Option, len(options))
	for i := range options {
		options[i].ID = r.nextOptionID
		r.nextOptionID++
		options[i].PollID = p.ID
		options[i].CreatedAt = now
		cloned[i] = options[i]
	}
	r.opts[p.ID] = cloned
	return p.ID, nil
}

func (r *testPollRepo) GetByID(ctx context.Context, id int64) (*poll.Poll, []poll.Option, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.polls[id]
	if !ok {
		return nil, nil, sql.ErrNoRows
	}
	opts := r.opts[id]
	copyPoll := *p
	copiedOpts := make([]poll.Option, len(opts))
	copy(copiedOpts, opts)
	return &copyPoll, copiedOpts, nil
}

func (r *testPollRepo) List(ctx context.Context, status *string) ([]poll.Poll, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	res := []poll.Poll{}
	for _, p := range r.polls {
		if status != nil && p.Status != *status {
			continue
		}
		res = append(res, *p)
	}
	return res, nil
}

func (r *testPollRepo) UpdateStatus(ctx context.Context, id int64, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.polls[id]
	if !ok {
		return sql.ErrNoRows
	}
	p.Status = status
	p.UpdatedAt = time.Now()
	return nil
}

func (r *testPollRepo) Update(ctx context.Context, id int64, input poll.UpdateInput) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.polls[id]
	if !ok {
		return sql.ErrNoRows
	}
	if input.Title != nil {
		p.Title = *input.Title
	}
	if input.Description.Set {
		p.Description = input.Description.Value
	}
	if input.StartsAt.Set {
		p.StartsAt = input.StartsAt.Value
	}
	if input.EndsAt.Set {
		p.EndsAt = input.EndsAt.Value
	}
	p.UpdatedAt = time.Now()
	return nil
}

func (r *testPollRepo) Delete(ctx context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.polls[id]; !ok {
		return sql.ErrNoRows
	}
	delete(r.polls, id)
	delete(r.opts, id)
	return nil
}

func (r *testPollRepo) optionBelongs(pollID, optionID int64) bool {
	opts := r.opts[pollID]
	for _, o := range opts {
		if o.ID == optionID {
			return true
		}
	}
	return false
}

type testVoteRepo struct {
	mu       sync.Mutex
	votes    map[int64]map[int64]int64
	agg      map[int64]map[int64]int64
	pollRepo *testPollRepo
}

func newTestVoteRepo(pollRepo *testPollRepo) *testVoteRepo {
	return &testVoteRepo{
		votes:    make(map[int64]map[int64]int64),
		agg:      make(map[int64]map[int64]int64),
		pollRepo: pollRepo,
	}
}

func (r *testVoteRepo) Create(ctx context.Context, v *vote.Vote) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.pollRepo.polls[v.PollID]
	if !ok {
		return vote.ErrPollNotFound
	}
	if p.Status != "active" {
		return vote.ErrPollNotActive
	}
	if !r.pollRepo.optionBelongs(v.PollID, v.OptionID) {
		return vote.ErrOptionNotInPoll
	}
	if _, ok := r.votes[v.PollID]; !ok {
		r.votes[v.PollID] = make(map[int64]int64)
	}
	if _, exists := r.votes[v.PollID][v.UserID]; exists {
		return vote.ErrAlreadyVoted
	}
	r.votes[v.PollID][v.UserID] = v.OptionID
	v.ID = int64(len(r.votes[v.PollID]))
	v.CreatedAt = time.Now()
	if p.Status != "active" {
		return vote.ErrPollNotActive
	}
	return nil
}

func (r *testVoteRepo) CountByPoll(ctx context.Context, pollID int64) (map[int64]int64, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := make(map[int64]int64)
	var total int64
	for _, optID := range r.votes[pollID] {
		m[optID]++
		total++
	}
	return m, total, nil
}

func (r *testVoteRepo) TotalVotes(ctx context.Context, pollID int64) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return int64(len(r.votes[pollID])), nil
}

func (r *testVoteRepo) AggregatedByPoll(ctx context.Context, pollID int64) (map[int64]int64, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	agg := r.agg[pollID]
	res := make(map[int64]int64)
	var total int64
	for opt, c := range agg {
		res[opt] = c
		total += c
	}
	return res, total, nil
}

func (r *testVoteRepo) IncrementAggregated(ctx context.Context, pollID, optionID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.agg[pollID]; !ok {
		r.agg[pollID] = make(map[int64]int64)
	}
	r.agg[pollID][optionID]++
	return nil
}

func (r *testVoteRepo) GetPollStatus(ctx context.Context, pollID int64) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.pollRepo.polls[pollID]
	if !ok {
		return "", sql.ErrNoRows
	}
	return p.Status, nil
}

func setupServer(t *testing.T) (*httptest.Server, *testUserRepo, *testPollRepo, *testVoteRepo, func()) {
	t.Helper()
	userRepo := newTestUserRepo()
	pollRepo := newTestPollRepo()
	voteRepo := newTestVoteRepo(pollRepo)

	userSvc := user.NewService(userRepo)
	pollSvc := poll.NewService(pollRepo)
	voteSvc := vote.NewService(voteRepo)
	jwtMgr := jwtpkg.NewManager("secret", "test-issuer")
	voteCh := make(chan worker.VoteEvent, 100)

	server := httptest.NewServer(NewRouter(userSvc, pollSvc, voteSvc, jwtMgr, voteRepo, voteCh, &sql.DB{}, nil))
	cleanup := func() {
		server.Close()
		close(voteCh)
	}
	return server, userRepo, pollRepo, voteRepo, cleanup
}

func seedUserWithPassword(t *testing.T, repo *testUserRepo, email, role, password string) int64 {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	repo.seed(&user.User{
		Email:        email,
		PasswordHash: string(hash),
		Role:         role,
		IsActive:     true,
	})
	return repo.byMail[email]
}

func loginAndToken(t *testing.T, serverURL, email, password string) string {
	t.Helper()
	body, _ := json.Marshal(authRequest{Email: email, Password: password})
	resp, err := http.Post(serverURL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status: %d", resp.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	token, _ := payload["token"].(string)
	if token == "" {
		t.Fatalf("token missing")
	}
	return token
}

func createPollViaAPI(t *testing.T, serverURL, token string, req createPollRequest) int64 {
	t.Helper()
	data, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest(http.MethodPost, serverURL+"/api/v1/polls", bytes.NewReader(data))
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("create poll request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var payload map[string]int64
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode create poll: %v", err)
	}
	return payload["id"]
}

func updatePollStatus(t *testing.T, serverURL, token string, pollID int64, status string) {
	t.Helper()
	body, _ := json.Marshal(updateStatusRequest{Status: status})
	req, _ := http.NewRequest(http.MethodPatch, serverURL+"/api/v1/polls/"+itoa(pollID)+"/status", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("update poll status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 status update, got %d", resp.StatusCode)
	}
}

func votePoll(t *testing.T, serverURL, token string, pollID, optionID int64) *http.Response {
	t.Helper()
	body, _ := json.Marshal(voteRequest{OptionID: optionID})
	req, _ := http.NewRequest(http.MethodPost, serverURL+"/api/v1/polls/"+itoa(pollID)+"/vote", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("vote request: %v", err)
	}
	return resp
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}

func decodeError(t *testing.T, resp *http.Response) map[string]string {
	t.Helper()
	var payload map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	return payload
}

func TestRBACForUserRole(t *testing.T) {
	server, userRepo, _, _, cleanup := setupServer(t)
	defer cleanup()

	seedUserWithPassword(t, userRepo, "admin@test.com", "admin", "pass123")
	seedUserWithPassword(t, userRepo, "user@test.com", "user", "pass123")

	adminToken := loginAndToken(t, server.URL, "admin@test.com", "pass123")
	userToken := loginAndToken(t, server.URL, "user@test.com", "pass123")

	// Ensure admin path works for admin
	createPollViaAPI(t, server.URL, adminToken, createPollRequest{
		Title:   "Admin poll",
		Options: []string{"yes", "no"},
	})

	// User token cannot create poll
	userPollReq := createPollRequest{Title: "User poll", Options: []string{"a", "b"}}
	body, _ := json.Marshal(userPollReq)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/polls", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+userToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("rbac create poll: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for user create poll, got %d", resp.StatusCode)
	}

	// User token cannot change roles
	roleReq := updateRoleRequest{Role: "admin"}
	payload, _ := json.Marshal(roleReq)
	roleHTTPReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/api/v1/users/1/role", bytes.NewReader(payload))
	roleHTTPReq.Header.Set("Authorization", "Bearer "+userToken)
	roleHTTPReq.Header.Set("Content-Type", "application/json")
	roleResp, err := http.DefaultClient.Do(roleHTTPReq)
	if err != nil {
		t.Fatalf("rbac update role: %v", err)
	}
	defer roleResp.Body.Close()
	if roleResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for role update, got %d", roleResp.StatusCode)
	}
}

func TestDeactivatedUserTokenRejected(t *testing.T) {
	server, userRepo, _, _, cleanup := setupServer(t)
	defer cleanup()

	seedUserWithPassword(t, userRepo, "admin@test.com", "admin", "pass123")
	userID := seedUserWithPassword(t, userRepo, "user@test.com", "user", "pass123")

	adminToken := loginAndToken(t, server.URL, "admin@test.com", "pass123")
	userToken := loginAndToken(t, server.URL, "user@test.com", "pass123")

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/polls", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("poll list request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 before deactivation, got %d", resp.StatusCode)
	}

	deactivateReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/api/v1/users/"+itoa(userID)+"/deactivate", nil)
	deactivateReq.Header.Set("Authorization", "Bearer "+adminToken)
	deactivateResp, err := http.DefaultClient.Do(deactivateReq)
	if err != nil {
		t.Fatalf("deactivate request: %v", err)
	}
	deactivateResp.Body.Close()
	if deactivateResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 for deactivate, got %d", deactivateResp.StatusCode)
	}

	req2, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/polls", nil)
	req2.Header.Set("Authorization", "Bearer "+userToken)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("poll list request after deactivation: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 after deactivation, got %d", resp2.StatusCode)
	}
	errPayload := decodeError(t, resp2)
	if errPayload["error"] != "inactive_user" {
		t.Fatalf("expected inactive_user error, got %q", errPayload["error"])
	}
}

func TestAdminUsersCRUD(t *testing.T) {
	server, userRepo, _, _, cleanup := setupServer(t)
	defer cleanup()

	adminID := seedUserWithPassword(t, userRepo, "admin@test.com", "admin", "pass123")
	userID := seedUserWithPassword(t, userRepo, "user@test.com", "user", "pass123")

	adminToken := loginAndToken(t, server.URL, "admin@test.com", "pass123")

	listReq, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/users", nil)
	listReq.Header.Set("Authorization", "Bearer "+adminToken)
	listResp, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatalf("list users request: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for list users, got %d", listResp.StatusCode)
	}

	var usersList []user.User
	if err := json.NewDecoder(listResp.Body).Decode(&usersList); err != nil {
		t.Fatalf("decode users list: %v", err)
	}
	if len(usersList) != 2 {
		t.Fatalf("expected 2 users, got %d", len(usersList))
	}
	emails := make(map[string]bool, len(usersList))
	for _, u := range usersList {
		emails[u.Email] = true
		if u.PasswordHash != "" {
			t.Fatalf("password hash must not be returned in API response")
		}
	}
	if !emails["admin@test.com"] || !emails["user@test.com"] {
		t.Fatalf("unexpected users list: %+v", usersList)
	}

	// update role user -> admin
	body, _ := json.Marshal(updateRoleRequest{Role: "admin"})
	roleReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/api/v1/users/"+itoa(userID)+"/role", bytes.NewReader(body))
	roleReq.Header.Set("Authorization", "Bearer "+adminToken)
	roleReq.Header.Set("Content-Type", "application/json")
	roleResp, err := http.DefaultClient.Do(roleReq)
	if err != nil {
		t.Fatalf("update role request: %v", err)
	}
	roleResp.Body.Close()
	if roleResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 for update role, got %d", roleResp.StatusCode)
	}

	// sanity check: cannot update the seeded admin to an invalid role
	badBody, _ := json.Marshal(updateRoleRequest{Role: "superadmin"})
	badRoleReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/api/v1/users/"+itoa(adminID)+"/role", bytes.NewReader(badBody))
	badRoleReq.Header.Set("Authorization", "Bearer "+adminToken)
	badRoleReq.Header.Set("Content-Type", "application/json")
	badRoleResp, err := http.DefaultClient.Do(badRoleReq)
	if err != nil {
		t.Fatalf("bad update role request: %v", err)
	}
	defer badRoleResp.Body.Close()
	if badRoleResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid role, got %d", badRoleResp.StatusCode)
	}
}

func TestPatchPollCanClearEndsAt(t *testing.T) {
	server, userRepo, _, _, cleanup := setupServer(t)
	defer cleanup()

	seedUserWithPassword(t, userRepo, "admin@test.com", "admin", "pass123")
	adminToken := loginAndToken(t, server.URL, "admin@test.com", "pass123")

	start := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	end := start.Add(1 * time.Hour)

	pollID := createPollViaAPI(t, server.URL, adminToken, createPollRequest{
		Title:    "Timed poll",
		StartsAt: strPtr(start.Format(time.RFC3339)),
		EndsAt:   strPtr(end.Format(time.RFC3339)),
		Options:  []string{"A", "B"},
	})

	patchReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/api/v1/polls/"+itoa(pollID), bytes.NewReader([]byte(`{"ends_at":null}`)))
	patchReq.Header.Set("Authorization", "Bearer "+adminToken)
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil {
		t.Fatalf("patch poll request: %v", err)
	}
	patchResp.Body.Close()
	if patchResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 for patch, got %d", patchResp.StatusCode)
	}

	getReq, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/polls/"+itoa(pollID), nil)
	getReq.Header.Set("Authorization", "Bearer "+adminToken)
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatalf("get poll request: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for get poll, got %d", getResp.StatusCode)
	}

	var payload pollDetailsResponse
	if err := json.NewDecoder(getResp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode poll: %v", err)
	}
	if payload.Poll == nil {
		t.Fatalf("expected poll payload")
	}
	if payload.Poll.EndsAt != nil {
		t.Fatalf("expected ends_at to be cleared, got %v", payload.Poll.EndsAt)
	}
}

func TestPollResultsNotFound(t *testing.T) {
	server, userRepo, _, _, cleanup := setupServer(t)
	defer cleanup()

	seedUserWithPassword(t, userRepo, "user@test.com", "user", "pass123")
	token := loginAndToken(t, server.URL, "user@test.com", "pass123")

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/polls/9999/results", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("results request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for results not found, got %d", resp.StatusCode)
	}
	errPayload := decodeError(t, resp)
	if errPayload["error"] != "poll_not_found" {
		t.Fatalf("expected poll_not_found error, got %q", errPayload["error"])
	}
}

func TestVoteIdempotencyAndConflicts(t *testing.T) {
	server, userRepo, pollRepo, _, cleanup := setupServer(t)
	defer cleanup()

	seedUserWithPassword(t, userRepo, "admin@test.com", "admin", "pass123")
	seedUserWithPassword(t, userRepo, "user@test.com", "user", "pass123")

	adminToken := loginAndToken(t, server.URL, "admin@test.com", "pass123")
	userToken := loginAndToken(t, server.URL, "user@test.com", "pass123")

	pollID := createPollViaAPI(t, server.URL, adminToken, createPollRequest{
		Title:   "Campus Survey",
		Options: []string{"yes", "no"},
	})
	updatePollStatus(t, server.URL, adminToken, pollID, "active")

	opts := pollRepo.opts[pollID]
	firstResp := votePoll(t, server.URL, userToken, pollID, opts[0].ID)
	defer firstResp.Body.Close()
	if firstResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 first vote, got %d", firstResp.StatusCode)
	}

	secondResp := votePoll(t, server.URL, userToken, pollID, opts[1].ID)
	defer secondResp.Body.Close()
	if secondResp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate vote, got %d", secondResp.StatusCode)
	}
}

func TestPollStatusGating(t *testing.T) {
	server, userRepo, pollRepo, _, cleanup := setupServer(t)
	defer cleanup()

	seedUserWithPassword(t, userRepo, "admin@test.com", "admin", "pass123")
	seedUserWithPassword(t, userRepo, "user@test.com", "user", "pass123")

	adminToken := loginAndToken(t, server.URL, "admin@test.com", "pass123")
	userToken := loginAndToken(t, server.URL, "user@test.com", "pass123")

	pollID := createPollViaAPI(t, server.URL, adminToken, createPollRequest{
		Title:   "Draft poll",
		Options: []string{"opt1", "opt2"},
	})
	opts := pollRepo.opts[pollID]

	resp := votePoll(t, server.URL, userToken, pollID, opts[0].ID)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for draft poll vote, got %d", resp.StatusCode)
	}

	updatePollStatus(t, server.URL, adminToken, pollID, "closed")
	respClosed := votePoll(t, server.URL, userToken, pollID, opts[0].ID)
	defer respClosed.Body.Close()
	if respClosed.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for closed poll vote, got %d", respClosed.StatusCode)
	}
}

func TestOptionMustBelongToPoll(t *testing.T) {
	server, userRepo, pollRepo, _, cleanup := setupServer(t)
	defer cleanup()

	seedUserWithPassword(t, userRepo, "admin@test.com", "admin", "pass123")
	seedUserWithPassword(t, userRepo, "user@test.com", "user", "pass123")

	adminToken := loginAndToken(t, server.URL, "admin@test.com", "pass123")
	userToken := loginAndToken(t, server.URL, "user@test.com", "pass123")

	pollA := createPollViaAPI(t, server.URL, adminToken, createPollRequest{
		Title:   "Poll A",
		Options: []string{"A1", "A2"},
	})
	pollB := createPollViaAPI(t, server.URL, adminToken, createPollRequest{
		Title:   "Poll B",
		Options: []string{"B1", "B2"},
	})
	updatePollStatus(t, server.URL, adminToken, pollA, "active")
	updatePollStatus(t, server.URL, adminToken, pollB, "active")

	optionFromB := pollRepo.opts[pollB][0].ID
	resp := votePoll(t, server.URL, userToken, pollA, optionFromB)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for option not in poll, got %d", resp.StatusCode)
	}
	errPayload := decodeError(t, resp)
	if errPayload["error"] == "" || errPayload["message"] == "" {
		t.Fatalf("expected structured error payload")
	}
}

func TestPollNotFoundPatchAndDelete(t *testing.T) {
	server, userRepo, _, _, cleanup := setupServer(t)
	defer cleanup()

	seedUserWithPassword(t, userRepo, "admin@test.com", "admin", "pass123")
	adminToken := loginAndToken(t, server.URL, "admin@test.com", "pass123")

	body, _ := json.Marshal(map[string]any{"title": "new title"})
	req, _ := http.NewRequest(http.MethodPatch, server.URL+"/api/v1/polls/9999", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("patch poll: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for patch not found, got %d", resp.StatusCode)
	}

	delReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/api/v1/polls/9999", nil)
	delReq.Header.Set("Authorization", "Bearer "+adminToken)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("delete poll: %v", err)
	}
	defer delResp.Body.Close()
	if delResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for delete not found, got %d", delResp.StatusCode)
	}
	errPayload := decodeError(t, delResp)
	if errPayload["error"] == "" {
		t.Fatalf("expected error code in response")
	}
}

func strPtr(s string) *string {
	return &s
}
