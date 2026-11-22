package main

// 회원 탈퇴는 두 단계다. DELETE /me는 delete_after(신청 시각 + withdrawGrace)만 찍고 세션을 끊는다.
// 그 사이 다시 로그인하면 POST /me/withdraw/cancel로 취소할 수 있고, 기한이 지나면
// schedulePurge가 purgeUser로 영구 삭제한다. 삭제 뒤 같은 카카오 계정으로 로그인하면 새 회원이 된다.

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	withdrawGrace = 7 * 24 * time.Hour
	purgeInterval = time.Hour
)

// deleteAfterExpr는 userCols와 AdminUser가 같이 쓴다. users 한 행만으로 계산돼야 RETURNING에도 쓸 수 있다.
const deleteAfterExpr = `to_char(delete_after AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS delete_after`

// purgeSQL은 순서가 중요하다. 마지막 DELETE FROM users가 나머지 개인 데이터(평가, 즐겨찾기,
// 팔로우, 담벼락 글, 신고, 인증 신청, 프로필 이미지, refresh token)를 CASCADE로 지우고,
// 평가·댓글 집계는 트리거가 맞춘다. 댓글만 FK가 SET NULL이라 그 전에 직접 정리한다.
var purgeSQL = []string{
	// 답글이 하나도 없는 댓글은 흔적 없이 지운다.
	`DELETE FROM comments c WHERE c.user_id = $1
	   AND NOT EXISTS (SELECT 1 FROM comments rp WHERE rp.parent_id = c.id)`,
	// 답글이 달린 댓글은 지우면 남의 답글까지 CASCADE로 사라지니 본문만 비운 삭제 표시로 남긴다.
	`UPDATE comments SET body = '', deleted_at = COALESCE(deleted_at, now()) WHERE user_id = $1`,
	// 인증 코드는 users가 아니라 이메일에 묶여 있어 CASCADE가 닿지 않는다.
	`DELETE FROM email_codes WHERE email = (SELECT email FROM users WHERE id = $1)`,
	`DELETE FROM users WHERE id = $1`,
}

// purgeUser는 회원과 그 데이터를 영구 삭제한다. 회원이 없으면 pgx.ErrNoRows.
func (s *server) purgeUser(ctx context.Context, id string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for i, q := range purgeSQL {
		tag, err := tx.Exec(ctx, q, id)
		if err != nil {
			return err
		}
		if i == len(purgeSQL)-1 && tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
	}
	return tx.Commit(ctx)
}

// DELETE /me — 탈퇴 신청. 갱신된 User(delete_after 포함)를 돌려주고 모든 기기의 세션을 끊는다.
// 이미 신청한 상태면 기한을 늘리지 않는다.
func (s *server) requestWithdraw(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer tx.Rollback(ctx)
	id := currentUser(r).ID
	var u User
	if err := tx.QueryRow(ctx,
		"UPDATE users SET delete_after = COALESCE(delete_after, now() + make_interval(secs => $2)) WHERE id = $1 RETURNING "+userCols,
		id, withdrawGrace.Seconds()).Scan(u.scanDest()...); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if _, err := tx.Exec(ctx, "UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL", id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, u)
}

// POST /me/withdraw/cancel — 유예 기간 안에 다시 로그인한 회원이 탈퇴를 취소한다.
func (s *server) cancelWithdraw(w http.ResponseWriter, r *http.Request) {
	var u User
	if err := s.db.QueryRow(r.Context(),
		"UPDATE users SET delete_after = NULL WHERE id = $1 RETURNING "+userCols, currentUser(r).ID).
		Scan(u.scanDest()...); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, u)
}

// DELETE /admin/users/{id} — 유예 기간 없이 바로 영구 삭제한다. 실수로 자기 계정을 지우지 않게 막는다.
func (s *server) adminPurgeUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == currentUser(r).ID {
		writeErr(w, 400, "자기 계정은 여기서 삭제할 수 없습니다")
		return
	}
	err := s.purgeUser(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, 404, "user not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

// schedulePurge는 기동 직후 한 번, 그 뒤 purgeInterval마다 기한이 지난 탈퇴 신청을 처리한다.
// 서버가 꺼져 있던 동안 지난 건도 다음 실행에서 함께 지운다.
func (s *server) schedulePurge(ctx context.Context) {
	t := time.NewTicker(purgeInterval)
	defer t.Stop()
	for {
		s.purgeExpired(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *server) purgeExpired(ctx context.Context) {
	rows, err := s.db.Query(ctx, "SELECT id FROM users WHERE delete_after <= now()")
	if err != nil {
		log.Printf("withdraw purge: 조회 실패: %v", err)
		return
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		log.Printf("withdraw purge: 조회 실패: %v", err)
		return
	}
	for _, id := range ids {
		// 한 명이 실패해도 나머지는 지운다. 실패한 회원은 다음 실행에서 다시 시도한다.
		if err := s.purgeUser(ctx, id); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("withdraw purge: %s 삭제 실패: %v", id, err)
			continue
		}
		log.Printf("withdraw purge: %s 삭제", id)
	}
}
