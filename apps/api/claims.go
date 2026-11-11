package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

const maxClaimNoteLen = 200

// ArtistClaim은 "아티스트 본인입니다" 신청 한 건이다. 사용자당 하나만 두고,
// 관리자가 approved로 바꾸면 담벼락에서 그 사용자의 이름이 아티스트명으로 표시된다.
type ArtistClaim struct {
	UserID            string  `json:"user_id" db:"user_id"`
	Nickname          string  `json:"nickname" db:"nickname"`
	ArtistID          string  `json:"artist_id" db:"artist_id"`
	ArtistName        *string `json:"artist_name" db:"artist_name"`
	ArtistDisplayName *string `json:"artist_display_name" db:"artist_display_name"`
	ArtistImage       *string `json:"artist_image_url" db:"artist_image_url"`
	Status            string  `json:"status" db:"status"` // pending | approved | rejected
	Note              string  `json:"note" db:"note"`
	CreatedAt         string  `json:"created_at" db:"created_at"`
	ReviewedAt        *string `json:"reviewed_at" db:"reviewed_at"`
}

const claimSelect = `SELECT c.user_id, u.nickname, c.artist_id, ar.name AS artist_name,
	ar.display_name AS artist_display_name, ar.image_url AS artist_image_url, c.status, c.note,
	to_char(c.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at,
	to_char(c.reviewed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS reviewed_at
	FROM artist_claims c JOIN users u ON u.id = c.user_id JOIN artists ar ON ar.id = c.artist_id`

func (s *server) getClaim(r *http.Request, userID string) (*ArtistClaim, error) {
	rows, err := s.db.Query(r.Context(), claimSelect+" WHERE c.user_id=$1", userID)
	if err != nil {
		return nil, err
	}
	c, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[ArtistClaim])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &c, err
}

// getMyClaim은 신청이 없으면 null을 준다.
func (s *server) getMyClaim(w http.ResponseWriter, r *http.Request) {
	c, err := s.getClaim(r, currentUser(r).ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, c)
}

// putMyClaim은 신청을 새로 넣거나 고친다. 아티스트나 메모가 바뀌면 다시 검수 대기로
// 돌아가서, 승인된 사용자가 다른 아티스트로 슬쩍 갈아탈 수 없다.
func (s *server) putMyClaim(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ArtistID string `json:"artist_id"`
		Note     string `json:"note"`
	}
	if !decode(w, r, &body) {
		return
	}
	artistID, note := strings.TrimSpace(body.ArtistID), strings.TrimSpace(body.Note)
	if artistID == "" {
		writeErr(w, 400, "artist_id is required")
		return
	}
	if len([]rune(note)) > maxClaimNoteLen {
		writeErr(w, 400, "note is too long (max 200 characters)")
		return
	}
	var exists bool
	if err := s.db.QueryRow(r.Context(), "SELECT EXISTS (SELECT 1 FROM artists WHERE id=$1)", artistID).Scan(&exists); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !exists {
		writeErr(w, 404, "artist not found")
		return
	}
	userID := currentUser(r).ID
	if _, err := s.db.Exec(r.Context(), `
		INSERT INTO artist_claims(user_id, artist_id, note) VALUES($1,$2,$3)
		ON CONFLICT(user_id) DO UPDATE SET artist_id=EXCLUDED.artist_id, note=EXCLUDED.note,
		  status='pending', created_at=now(), reviewed_at=NULL
		WHERE artist_claims.artist_id <> EXCLUDED.artist_id OR artist_claims.note <> EXCLUDED.note
		   OR artist_claims.status = 'rejected'`,
		userID, artistID, note); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	c, err := s.getClaim(r, userID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, c)
}

// deleteMyClaim은 신청을 철회한다. 승인된 상태였다면 인증 표시도 함께 사라진다.
func (s *server) deleteMyClaim(w http.ResponseWriter, r *http.Request) {
	if _, err := s.db.Exec(r.Context(), "DELETE FROM artist_claims WHERE user_id=$1", currentUser(r).ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

// adminListClaims: ?status=pending(기본)|approved|rejected. 대기 건은 오래된 순, 나머지는 최근 검수 순.
func (s *server) adminListClaims(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	order := " ORDER BY c.created_at"
	switch status {
	case "approved", "rejected":
		order = " ORDER BY c.reviewed_at DESC NULLS LAST"
	default:
		status = "pending"
	}
	rows, err := s.db.Query(r.Context(), claimSelect+" WHERE c.status=$1"+order+" LIMIT 200", status)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	claims, err := pgx.CollectRows(rows, pgx.RowToStructByName[ArtistClaim])
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if claims == nil {
		claims = []ArtistClaim{}
	}
	writeJSON(w, 200, claims)
}

// adminReviewClaim은 승인/거절한다. artist_id를 주면 신청자가 고른 행 대신 그 아티스트로
// 매핑한다 — 같은 사람이 Spotify·Apple 행으로 갈라져 있을 때 관리자가 맞는 쪽을 고른다.
func (s *server) adminReviewClaim(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Status   string  `json:"status"`
		ArtistID *string `json:"artist_id"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Status != "approved" && body.Status != "rejected" {
		writeErr(w, 400, "status must be approved or rejected")
		return
	}
	artistID := nilIfBlank(body.ArtistID)
	if artistID != nil {
		var exists bool
		if err := s.db.QueryRow(r.Context(), "SELECT EXISTS (SELECT 1 FROM artists WHERE id=$1)", *artistID).Scan(&exists); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if !exists {
			writeErr(w, 404, "artist not found")
			return
		}
	}
	userID := r.PathValue("userId")
	tag, err := s.db.Exec(r.Context(),
		"UPDATE artist_claims SET status=$2, artist_id=COALESCE($3, artist_id), reviewed_at=now() WHERE user_id=$1",
		userID, body.Status, artistID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, 404, "claim not found")
		return
	}
	c, err := s.getClaim(r, userID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, c)
}
