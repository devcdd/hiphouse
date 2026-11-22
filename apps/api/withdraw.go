package main

import "net/http"

// withdrawSQL은 순서가 중요하다. 마지막 DELETE FROM users가 나머지 개인 데이터(평가, 즐겨찾기,
// 팔로우, 담벼락 글, 신고, 인증 신청, 프로필 이미지, refresh token)를 CASCADE로 지우고,
// 평가·댓글 집계는 트리거가 맞춘다. 댓글만 FK가 SET NULL이라 그 전에 직접 정리한다.
var withdrawSQL = []string{
	// 답글이 하나도 없는 댓글은 흔적 없이 지운다.
	`DELETE FROM comments c WHERE c.user_id = $1
	   AND NOT EXISTS (SELECT 1 FROM comments rp WHERE rp.parent_id = c.id)`,
	// 답글이 달린 댓글은 지우면 남의 답글까지 CASCADE로 사라지니 본문만 비운 삭제 표시로 남긴다.
	`UPDATE comments SET body = '', deleted_at = COALESCE(deleted_at, now()) WHERE user_id = $1`,
	// 인증 코드는 users가 아니라 이메일에 묶여 있어 CASCADE가 닿지 않는다.
	`DELETE FROM email_codes WHERE email = (SELECT email FROM users WHERE id = $1)`,
	`DELETE FROM users WHERE id = $1`,
}

// DELETE /me — 회원 탈퇴. 되돌릴 수 없다. 카카오로 다시 로그인하면 같은 카카오 id로 새 회원이 된다.
func (s *server) withdraw(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer tx.Rollback(ctx)
	id := currentUser(r).ID
	for _, q := range withdrawSQL {
		if _, err := tx.Exec(ctx, q, id); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}
