package auth

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"magu/internal/pgerr"
)

var (
	ErrUsernameTaken = errors.New("username sudah dipakai")
	ErrUserNotFound  = errors.New("pengguna tidak ditemukan")
)

type Account struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	FullName  string    `json:"full_name"`
	Role      string    `json:"role"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Service) ListUsers(ctx context.Context) ([]Account, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, username, full_name, role, is_active, created_at FROM users ORDER BY is_active DESC, full_name`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Account])
}

func (s *Service) CreateUser(ctx context.Context, a *Account, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	err = s.db.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, full_name, role, is_active)
		VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`,
		a.Username, string(hash), a.FullName, a.Role, a.IsActive).Scan(&a.ID, &a.CreatedAt)
	if pgerr.IsUnique(err) {
		return ErrUsernameTaken
	}
	return err
}

// UpdateUser mengganti password hanya bila password tidak kosong.
func (s *Service) UpdateUser(ctx context.Context, a *Account, password string) error {
	var hash *string
	if password != "" {
		b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		h := string(b)
		hash = &h
	}
	err := s.db.QueryRow(ctx, `
		UPDATE users
		SET username = $2, full_name = $3, role = $4, is_active = $5, password_hash = COALESCE($6, password_hash)
		WHERE id = $1
		RETURNING created_at`, a.ID, a.Username, a.FullName, a.Role, a.IsActive, hash).Scan(&a.CreatedAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrUserNotFound
	case pgerr.IsUnique(err):
		return ErrUsernameTaken
	}
	return err
}

// RevokeSessions mengeluarkan seorang pengguna dari semua perangkat, mis. setelah dinonaktifkan atau ganti password.
func (s *Service) RevokeSessions(ctx context.Context, userID int64) error {
	key := userSessionsKey(userID)
	tokens, err := s.cache.SMembers(ctx, key).Result()
	if err != nil {
		return err
	}
	keys := []string{key}
	for _, token := range tokens {
		keys = append(keys, sessionKey(token))
	}
	return s.cache.Del(ctx, keys...).Err()
}

func userSessionsKey(userID int64) string {
	return "user_sessions:" + strconv.FormatInt(userID, 10)
}
