# apps/api — hiphouse CRUD API

크롤러가 채운 Postgres(`artists`/`albums`)를 CRUD 하는 Go 서버. 의존성은 `pgx` 하나, 라우팅은 Go 표준 `net/http` mux.

## 실행

```sh
docker compose up -d --wait db        # 루트에서, Postgres 먼저
go run ./apps/api                      # → http://localhost:8080
```

`DATABASE_URL`(기본 `postgres://hiphouse:hiphouse@localhost:5432/hiphouse`) / `PORT`(기본 `8080`) 환경변수로 덮어쓰기.

## Swagger

- UI: http://localhost:8080/swagger/
- 스펙: http://localhost:8080/openapi.json

## 엔드포인트

| Method | Path | 설명 |
|---|---|---|
| GET | `/albums` | 목록. `?year=&artist_id=&limit=&offset=` |
| POST | `/albums` | 생성 (id 중복 시 409) |
| GET | `/albums/{id}` | 단건 |
| PUT | `/albums/{id}` | 수정 (전체 교체) |
| DELETE | `/albums/{id}` | 삭제 |
| GET | `/albums/{id}/tracks` | 앨범 트랙 목록 (동기화 전이면 `[]`) |
| GET/POST/GET/PUT/DELETE | `/artists`, `/artists/{id}` | 위와 동일 |
| GET | `/admin/users` | 회원 목록. `?q=&limit=&offset=` (관리자) |
| GET | `/admin/tracks/status` | 트랙 동기화 현황 (관리자) |
| POST | `/admin/tracks/backfill` | 트랙 미동기화 앨범 배치 백필 (관리자) |
| GET | `/healthz` | 헬스체크 |

## 테스트

```sh
go test ./apps/api    # buildAlbumListQuery self-check (DB 불필요)
```
