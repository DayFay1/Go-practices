package user

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"
)

type memoryUserRepo struct {
	mu     sync.Mutex
	users  map[int64]*User
	byMail map[string]int64
	nextID int64
}

func newMemoryUserRepo() *memoryUserRepo {
	return &memoryUserRepo{
		users:  make(map[int64]*User),
		byMail: make(map[string]int64),
		nextID: 1,
	}
}

func (r *memoryUserRepo) Create(ctx context.Context, u *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u.ID = r.nextID
	r.nextID++
	u.CreatedAt = time.Now()
	copyUser := *u
	r.users[u.ID] = &copyUser
	r.byMail[u.Email] = u.ID
	return nil
}

func (r *memoryUserRepo) GetByEmail(ctx context.Context, email string) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byMail[email]
	if !ok {
		return nil, sql.ErrNoRows
	}
	copyUser := *r.users[id]
	return &copyUser, nil
}

func (r *memoryUserRepo) GetByID(ctx context.Context, id int64) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	copyUser := *u
	return &copyUser, nil
}

func (r *memoryUserRepo) List(ctx context.Context) ([]User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	res := make([]User, 0, len(r.users))
	for _, u := range r.users {
		res = append(res, *u)
	}
	return res, nil
}

func (r *memoryUserRepo) UpdateRole(ctx context.Context, id int64, role string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return sql.ErrNoRows
	}
	u.Role = role
	return nil
}

func (r *memoryUserRepo) Deactivate(ctx context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return sql.ErrNoRows
	}
	u.IsActive = false
	return nil
}

func TestRegisterAndLogin(t *testing.T) {
	repo := newMemoryUserRepo()
	svc := NewService(repo)
	ctx := context.Background()

	u, err := svc.Register(ctx, "john@example.com", "s3cret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.Role != "user" {
		t.Fatalf("expected role user, got %s", u.Role)
	}
	if u.PasswordHash == "s3cret" || u.PasswordHash == "" {
		t.Fatalf("password should be hashed")
	}

	if _, err := svc.Login(ctx, "john@example.com", "s3cret"); err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if _, err := svc.Register(ctx, "john@example.com", "another"); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("expected email taken error")
	}
	if _, err := svc.Login(ctx, "john@example.com", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials error")
	}

	if err := svc.Deactivate(ctx, u.ID); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if _, err := svc.Login(ctx, "john@example.com", "s3cret"); !errors.Is(err, ErrInactiveUser) {
		t.Fatalf("expected inactive user error")
	}
}
func TestLoginUserNotFound(t *testing.T) {
	repo := newMemoryUserRepo()
	svc := NewService(repo)
	ctx := context.Background()

	_, err := svc.Login(ctx, "ghost@example.com", "password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials error, got %v", err)
	}
}
