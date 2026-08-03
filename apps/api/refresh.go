package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	accessTokenTTL  = time.Hour
	refreshTokenTTL = 30 * 24 * time.Hour
)

// newRefreshToken returns the token handed to the client and the hash stored in
// the DB. Only the hash is persisted: a leaked database dump then can't be
// replayed as a session.
func newRefreshToken() (token, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(buf)
	return token, hashToken(token), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// issueSession mints an access token plus a fresh refresh token row.
func (s *server) issueSession(ctx context.Context, u User) (access, refresh string, err error) {
	access, err = s.issueToken(u)
	if err != nil {
		return "", "", err
	}
	refresh, hash, err := newRefreshToken()
	if err != nil {
		return "", "", err
	}
	_, err = s.db.Exec(ctx,
		"INSERT INTO refresh_tokens(user_id, token_hash, expires_at) VALUES($1,$2,$3)",
		u.ID, hash, time.Now().Add(refreshTokenTTL))
	if err != nil {
		return "", "", err
	}
	return access, refresh, nil
}

// refreshSession rotates a refresh token: the presented one is revoked and a new
// pair is issued. Presenting an ALREADY revoked token means the client's token
// leaked and someone is replaying it, so every session for that user is killed.
func (s *server) refreshSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.RefreshToken == "" {
		writeErr(w, 400, "refresh_token is required")
		return
	}

	var (
		userID    string
		expiresAt time.Time
		revoked   bool
	)
	err := s.db.QueryRow(r.Context(),
		"SELECT user_id, expires_at, revoked_at IS NOT NULL FROM refresh_tokens WHERE token_hash=$1",
		hashToken(body.RefreshToken)).Scan(&userID, &expiresAt, &revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, 401, "invalid refresh token")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if revoked {
		// Token reuse — assume theft and force a fresh login everywhere.
		if _, err := s.db.Exec(r.Context(),
			"UPDATE refresh_tokens SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL", userID); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeErr(w, 401, "refresh token reused — all sessions revoked")
		return
	}
	if time.Now().After(expiresAt) {
		writeErr(w, 401, "refresh token expired")
		return
	}

	var u User
	if err := s.db.QueryRow(r.Context(), "SELECT "+userCols+" FROM users WHERE id=$1", userID).
		Scan(&u.ID, &u.Nickname, &u.Role, &u.ProfilePublic); err != nil {
		writeErr(w, 401, "user not found")
		return
	}

	// Revoke first: if issuing the new pair fails, the old token is already dead
	// rather than left usable alongside a half-created session.
	if _, err := s.db.Exec(r.Context(),
		"UPDATE refresh_tokens SET revoked_at=now() WHERE token_hash=$1", hashToken(body.RefreshToken)); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	access, refresh, err := s.issueSession(r.Context(), u)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"token": access, "refresh_token": refresh, "user": u})
}

// logout revokes the presented refresh token. No auth header required — holding
// the token is the proof, and a logout that fails because the access token
// already expired would be useless.
func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.RefreshToken != "" {
		if _, err := s.db.Exec(r.Context(),
			"UPDATE refresh_tokens SET revoked_at=now() WHERE token_hash=$1 AND revoked_at IS NULL",
			hashToken(body.RefreshToken)); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	w.WriteHeader(204)
}
