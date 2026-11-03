package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("username atau password salah")
	ErrNoSession          = errors.New("sesi tidak ditemukan")
)

const sessionTTL = 12 * time.Hour

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	FullName string `json:"full_name"`
	Role     string `json:"role"`
}

type Service struct {
	db    *pgxpool.Pool
	cache *redis.Client
}

func NewService(db *pgxpool.Pool, cache *redis.Client) *Service {
	return &Service{db: db, cache: cache}
}

func (s *Service) Login(ctx context.Context, username, password string) (string, *User, error) {
	var u User
	var hash string
	err := s.db.QueryRow(ctx, `
		SELECT id, username, full_name, role, password_hash
		FROM users WHERE username = $1 AND is_active`, username).
		Scan(&u.ID, &u.Username, &u.FullName, &u.Role, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, ErrInvalidCredentials
	}
	if err != nil {
		return "", nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", nil, ErrInvalidCredentials
	}

	raw := make([]byte, 32)
	rand.Read(raw)
	token := hex.EncodeToString(raw)

	data, err := json.Marshal(u)
	if err != nil {
		return "", nil, err
	}
	pipe := s.cache.TxPipeline()
	pipe.Set(ctx, sessionKey(token), data, sessionTTL)
	pipe.SAdd(ctx, userSessionsKey(u.ID), token)
	pipe.Expire(ctx, userSessionsKey(u.ID), sessionTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", nil, err
	}
	return token, &u, nil
}

// Session juga memperpanjang masa berlaku sesi, jadi kasir yang aktif tidak terlempar keluar di tengah shift.
func (s *Service) Session(ctx context.Context, token string) (*User, error) {
	if token == "" {
		return nil, ErrNoSession
	}
	data, err := s.cache.GetEx(ctx, sessionKey(token), sessionTTL).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrNoSession
	}
	if err != nil {
		return nil, err
	}
	var u User
	if err := json.Unmarshal(data, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	return s.cache.Del(ctx, sessionKey(token)).Err()
}

func sessionKey(token string) string {
	return "session:" + token
}

// SaveUser membuat user baru, atau mengganti password, nama, dan role bila username sudah ada.
func SaveUser(ctx context.Context, db *pgxpool.Pool, username, password, fullName, role string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `
		INSERT INTO users (username, password_hash, full_name, role)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (username) DO UPDATE
		SET password_hash = EXCLUDED.password_hash, full_name = EXCLUDED.full_name,
		    role = EXCLUDED.role, is_active = TRUE`, username, string(hash), fullName, role)
	return err
}
