package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type User struct {
	ID       string `json:"id" db:"id"`
	Nickname string `json:"nickname" db:"nickname"`
	Role     string `json:"role" db:"role"`
	// ProfilePublic is the single switch for "남한테 내 활동 보이기": off hides
	// everything on the public profile (see users.go). Sent on GET/PUT /me and,
	// so the profile page can explain itself, on GET /users/{id} too.
	ProfilePublic bool `json:"profile_public" db:"profile_public"`
	// NicknameSet is false until the user saves a nickname themselves; the web
	// app uses it to show the first-login greeting + nickname setup.
	NicknameSet bool `json:"nickname_set" db:"nickname_set"`
}

// userCols is the shape every User scan expects; keep it in step with the Scan
// argument order at each call site.
const userCols = "id,nickname,role,profile_public,nickname_set"

type ctxKey string

const userKey ctxKey = "user"

// --- JWT ---

func (s *server) issueToken(u User) (string, error) {
	claims := jwt.MapClaims{
		"sub":  u.ID,
		"role": u.Role,
		"exp":  time.Now().Add(accessTokenTTL).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
}

func (s *server) parseToken(tok string) (id, role string, err error) {
	t, err := jwt.Parse(tok, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil || !t.Valid {
		return "", "", errors.New("invalid token")
	}
	claims, _ := t.Claims.(jwt.MapClaims)
	id, _ = claims["sub"].(string)
	role, _ = claims["role"].(string)
	if id == "" {
		return "", "", errors.New("invalid token")
	}
	return id, role, nil
}

// --- Middleware ---

func (s *server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		id, role, err := s.parseToken(tok)
		if err != nil {
			writeErr(w, 401, "unauthorized")
			return
		}
		ctx := context.WithValue(r.Context(), userKey, User{ID: id, Role: role})
		next(w, r.WithContext(ctx))
	}
}

func (s *server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r).Role != "admin" {
			writeErr(w, 403, "admin only")
			return
		}
		next(w, r)
	})
}

func currentUser(r *http.Request) User {
	u, _ := r.Context().Value(userKey).(User)
	return u
}

// isAdminReq checks the bearer token without requiring it (used on public reads
// to optionally widen results for admins).
func (s *server) isAdminReq(r *http.Request) bool {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	_, role, err := s.parseToken(tok)
	return err == nil && role == "admin"
}

// callerID는 같은 방식으로 신원만 본다. 로그인한 사용자의 id, 토큰이 없거나 잘못됐으면
// "". 공개 읽기에서 주인 본인에게만 가려진 내용을 열어 주려고 쓴다 — 이걸 requireAuth로
// 바꾸면 비로그인 방문자가 프로필을 아예 못 본다.
func (s *server) callerID(r *http.Request) string {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	id, _, err := s.parseToken(tok)
	if err != nil {
		return ""
	}
	return id
}

// --- Kakao OAuth ---

type kakaoUser struct {
	ID           int64 `json:"id"`
	KakaoAccount struct {
		Profile struct {
			Nickname string `json:"nickname"`
		} `json:"profile"`
	} `json:"kakao_account"`
}

// exchangeKakao trades an auth code for the Kakao user's id + nickname.
func (s *server) exchangeKakao(ctx context.Context, code, redirectURI string) (kakaoUser, error) {
	form := url.Values{
		"grant_type":   {"authorization_code"},
		"client_id":    {s.kakaoKey},
		"redirect_uri": {redirectURI},
		"code":         {code},
	}
	if s.kakaoSecret != "" {
		form.Set("client_secret", s.kakaoSecret)
	}
	tokRes, err := kakaoPost(ctx, "https://kauth.kakao.com/oauth/token", form, "")
	if err != nil {
		return kakaoUser{}, err
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(tokRes, &tok); err != nil || tok.AccessToken == "" {
		return kakaoUser{}, errors.New("kakao token exchange failed")
	}
	meRes, err := kakaoPost(ctx, "https://kapi.kakao.com/v2/user/me", nil, tok.AccessToken)
	if err != nil {
		return kakaoUser{}, err
	}
	var ku kakaoUser
	if err := json.Unmarshal(meRes, &ku); err != nil || ku.ID == 0 {
		return kakaoUser{}, errors.New("kakao profile fetch failed")
	}
	return ku, nil
}

func kakaoPost(ctx context.Context, endpoint string, form url.Values, bearer string) ([]byte, error) {
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	buf, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		return nil, errors.New("kakao API " + strconv.Itoa(res.StatusCode) + ": " + string(buf))
	}
	return buf, nil
}

// --- Handlers ---

// loginKakao: body {code, redirect_uri} -> upsert user, return {token, user}.
func (s *server) loginKakao(w http.ResponseWriter, r *http.Request) {
	if s.kakaoKey == "" {
		writeErr(w, 500, "kakao login not configured (set KAKAO_REST_API_KEY)")
		return
	}
	var body struct {
		Code        string `json:"code"`
		RedirectURI string `json:"redirect_uri"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Code == "" || body.RedirectURI == "" {
		writeErr(w, 400, "code and redirect_uri are required")
		return
	}
	ku, err := s.exchangeKakao(r.Context(), body.Code, body.RedirectURI)
	if err != nil {
		writeErr(w, 401, err.Error())
		return
	}

	id := strconv.FormatInt(ku.ID, 10)
	nickname := ku.KakaoAccount.Profile.Nickname
	role := "user"
	if s.adminIDs[id] {
		role = "admin"
	}
	// Upsert: nickname is set on first login only; a returning user keeps their
	// (possibly self-edited via PUT /me) nickname. Role is refreshed from the env
	// allowlist but never demoted here.
	_, err = s.db.Exec(r.Context(),
		`INSERT INTO users(id,nickname,role) VALUES($1,$2,$3)
		 ON CONFLICT(id) DO UPDATE SET
		   role=CASE WHEN $3='admin' THEN 'admin' ELSE users.role END`,
		id, nickname, role)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	var u User
	if err := s.db.QueryRow(r.Context(), "SELECT "+userCols+" FROM users WHERE id=$1", id).
		Scan(&u.ID, &u.Nickname, &u.Role, &u.ProfilePublic, &u.NicknameSet); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	token, refresh, err := s.issueSession(r.Context(), u)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"token": token, "refresh_token": refresh, "user": u})
}

func (s *server) me(w http.ResponseWriter, r *http.Request) {
	var u User
	if err := s.db.QueryRow(r.Context(), "SELECT "+userCols+" FROM users WHERE id=$1", currentUser(r).ID).
		Scan(&u.ID, &u.Nickname, &u.Role, &u.ProfilePublic, &u.NicknameSet); err != nil {
		writeErr(w, 404, "user not found")
		return
	}
	writeJSON(w, 200, u)
}

// validateNickname trims and bounds a nickname; returns (clean, errMsg).
// Pure so it can be unit-tested without a DB.
func validateNickname(raw string) (string, string) {
	n := strings.TrimSpace(raw)
	switch {
	case n == "":
		return "", "nickname is required"
	case utf8.RuneCountInString(n) > 20:
		return "", "nickname too long (max 20)"
	}
	return n, ""
}

// updateMe lets the signed-in user change their own settings (PUT /me).
// Both fields are pointers so a caller can send just one: the nickname editor and
// the 공개 설정 토글 are separate screens and neither knows the other's value.
func (s *server) updateMe(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Nickname      *string `json:"nickname"`
		ProfilePublic *bool   `json:"profile_public"`
	}
	if !decode(w, r, &body) {
		return
	}
	var nick *string
	if body.Nickname != nil {
		clean, msg := validateNickname(*body.Nickname)
		if msg != "" {
			writeErr(w, 400, msg)
			return
		}
		nick = &clean
	}
	var u User
	// nickname_set: 닉네임을 실제로 보낸 요청에서만 켜지고, 한 번 켜지면 내려가지
	// 않는다 — 공개 설정만 바꾸는 PUT이 온보딩 상태를 건드리면 안 된다.
	err := s.db.QueryRow(r.Context(),
		"UPDATE users SET nickname=COALESCE($2,nickname), nickname_set=nickname_set OR $2 IS NOT NULL, "+
			"profile_public=COALESCE($3,profile_public) WHERE id=$1 RETURNING "+userCols,
		currentUser(r).ID, nick, body.ProfilePublic).
		Scan(&u.ID, &u.Nickname, &u.Role, &u.ProfilePublic, &u.NicknameSet)
	// 중복은 DB의 부분 유니크 인덱스가 잡는다. 미리 SELECT로 확인하면 확인과 UPDATE
	// 사이에 남이 같은 이름을 채가는 경합이 남는다.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		writeErr(w, 409, "이미 사용 중인 닉네임입니다")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, u)
}
