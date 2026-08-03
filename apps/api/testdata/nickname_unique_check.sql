-- 닉네임 중복 방지 마이그레이션 검증. 실제 개발 DB를 건드리지 않도록 스크래치 DB에서 돈다.
--
--   docker exec -i hiphouse-db psql -U hiphouse -d postgres \
--     -c 'DROP DATABASE IF EXISTS nickname_check' -c 'CREATE DATABASE nickname_check'
--   docker exec -i hiphouse-db psql -U hiphouse -d nickname_check -v ON_ERROR_STOP=1 \
--     < apps/api/testdata/nickname_unique_check.sql
--   docker exec -i hiphouse-db psql -U hiphouse -d postgres -c 'DROP DATABASE nickname_check'
--
-- main.go ensureAuthSchema 의 users 구문을 복사해 온 것이므로, 그쪽을 고치면 여기도 같이 고친다.
\set ON_ERROR_STOP on

CREATE TABLE users (
	id TEXT PRIMARY KEY,
	nickname TEXT NOT NULL DEFAULT '',
	role TEXT NOT NULL DEFAULT 'user',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 기존 유저: 중복 2쌍(대소문자 다른 것 포함) + 빈 닉네임
INSERT INTO users(id, nickname, created_at) VALUES
	('1000000001', 'hiphop',  now() - interval '3 day'),
	('1000000002', 'HIPHOP',  now() - interval '2 day'),
	('1000000003', 'hiphop',  now() - interval '1 day'),
	('1000000004', 'solo',    now()),
	('1000000005', '',        now()),
	('1000000006', '',        now());

-- === main.go ensureAuthSchema 의 users 관련 구문 그대로 ===
ALTER TABLE IF EXISTS users ADD COLUMN IF NOT EXISTS profile_public BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE IF EXISTS users ADD COLUMN IF NOT EXISTS nickname_set BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE IF EXISTS users ALTER COLUMN nickname_set SET DEFAULT false;
UPDATE users u SET nickname = left(u.nickname, 15) || '-' || right(u.id, 4)
FROM (
	SELECT id, row_number() OVER (PARTITION BY lower(nickname) ORDER BY created_at, id) AS rn
	FROM users WHERE nickname_set AND nickname <> ''
) d
WHERE u.id = d.id AND d.rn > 1;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_nickname
	ON users (lower(nickname)) WHERE nickname_set AND nickname <> '';
-- === 여기까지 ===

-- 1) 기존 유저는 nickname_set=true 로 백필 (재온보딩 없음)
DO $$ BEGIN
	ASSERT (SELECT count(*) FROM users WHERE NOT nickname_set) = 0, '기존 유저 백필 실패';
END $$;

-- 2) 먼저 가입한 사람이 원본 유지, 나머지만 접미사
DO $$ BEGIN
	ASSERT (SELECT nickname FROM users WHERE id='1000000001') = 'hiphop', '최초 가입자 닉네임이 바뀜';
	ASSERT (SELECT nickname FROM users WHERE id='1000000002') = 'HIPHOP-0002', '대소문자 중복 미정리';
	ASSERT (SELECT nickname FROM users WHERE id='1000000003') = 'hiphop-0003', '중복 미정리';
	ASSERT (SELECT nickname FROM users WHERE id='1000000004') = 'solo', '중복 아닌데 바뀜';
END $$;

-- 3) 빈 닉네임 여러 개는 인덱스가 무시 (부분 인덱스 WHERE nickname <> '')
DO $$ BEGIN
	ASSERT (SELECT count(*) FROM users WHERE nickname = '') = 2, '빈 닉네임이 건드려짐';
END $$;

-- 4) 재실행 멱등성: 두 번째 부팅에서 닉네임이 또 바뀌면 안 된다
UPDATE users u SET nickname = left(u.nickname, 15) || '-' || right(u.id, 4)
FROM (
	SELECT id, row_number() OVER (PARTITION BY lower(nickname) ORDER BY created_at, id) AS rn
	FROM users WHERE nickname_set AND nickname <> ''
) d
WHERE u.id = d.id AND d.rn > 1;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_nickname
	ON users (lower(nickname)) WHERE nickname_set AND nickname <> '';
DO $$ BEGIN
	ASSERT (SELECT nickname FROM users WHERE id='1000000003') = 'hiphop-0003', '재부팅 시 닉네임이 또 바뀜';
END $$;

-- 5) 첫 로그인 INSERT: 카카오 이름이 이미 쓰이는 값이어도 막히면 안 된다
INSERT INTO users(id, nickname, role) VALUES ('2000000001', 'hiphop', 'user');
DO $$ BEGIN
	ASSERT (SELECT nickname_set FROM users WHERE id='2000000001') = false, '신규 유저가 nickname_set=true 로 들어옴';
END $$;

-- 6) 온보딩 저장(PUT /me)에서 중복이면 23505 (updateMe 의 UPDATE 문 그대로)
DO $$ BEGIN
	UPDATE users SET nickname=COALESCE('HipHop',nickname), nickname_set=nickname_set OR 'HipHop' IS NOT NULL,
	                 profile_public=COALESCE(NULL,profile_public) WHERE id='2000000001';
	RAISE EXCEPTION '중복 닉네임이 통과됨';
EXCEPTION WHEN unique_violation THEN
	RAISE NOTICE 'ok: 중복 닉네임 23505';
END $$;

-- 7) 공개 설정만 바꾸는 PUT /me 는 nickname_set 을 켜지 않는다 (온보딩 상태 유지)
UPDATE users SET nickname=COALESCE(NULL,nickname), nickname_set=nickname_set OR NULL IS NOT NULL,
                 profile_public=COALESCE(false,profile_public) WHERE id='2000000001';
DO $$ BEGIN
	ASSERT (SELECT nickname_set FROM users WHERE id='2000000001') = false, '공개 설정 변경이 온보딩을 끝내버림';
	ASSERT (SELECT profile_public FROM users WHERE id='2000000001') = false, '공개 설정이 안 바뀜';
END $$;

-- 8) 고유하면 저장되고 nickname_set 이 켜진다
UPDATE users SET nickname=COALESCE('hiphop2',nickname), nickname_set=nickname_set OR 'hiphop2' IS NOT NULL,
                 profile_public=COALESCE(NULL,profile_public) WHERE id='2000000001';
DO $$ BEGIN
	ASSERT (SELECT nickname_set FROM users WHERE id='2000000001'), '고유 닉네임 저장 실패';
END $$;

SELECT '=== ALL CHECKS PASSED ===' AS result;
