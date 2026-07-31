import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Download, Search, X } from 'lucide-react'
import { Avatar } from '@/shared/ui/Avatar'
import { useDebouncedValue } from '@/shared/lib/useDebouncedValue'
import { searchSpotifyAlbums, searchSpotifyArtists } from '../api/spotifyAdminApi'
import { crawlAlbum, crawlArtist, fetchSpotifyKeys } from '@/features/crawl-artist'
import styles from './AdminPage.module.css'

const nf = new Intl.NumberFormat('ko-KR')

type Mode = 'artist' | 'album'

// 웹에서 바로 크롤링: Spotify를 검색해 저장한다 (artist.sh의 웹 버전).
// 아티스트 모드는 고른 아티스트의 전체 앨범을, 앨범 모드는 고른 앨범 1개를 수집한다.
// 앨범 검색에는 장르 필터가 없으므로(Spotify가 앨범 단위 장르를 주지 않음) 관리자가 직접 선별한다.
export function CrawlTab() {
  const qc = useQueryClient()
  // Configured credential pairs on the server (first/second/third/... env pairs).
  const keysQuery = useQuery({ queryKey: ['spotify-keys'], queryFn: fetchSpotifyKeys, staleTime: Infinity })
  const keys = keysQuery.data ?? []
  const [picked, setPicked] = useState('')
  const key = picked && keys.includes(picked) ? picked : (keys[0] ?? '') // '' = server default pair

  const [mode, setMode] = useState<Mode>('artist')
  const [input, setInput] = useState('')
  const q = useDebouncedValue(input.trim(), 350)
  const [message, setMessage] = useState('')

  const artistSearch = useQuery({
    queryKey: ['spotify-search', q, key],
    queryFn: () => searchSpotifyArtists(q, key),
    enabled: mode === 'artist' && q.length > 0,
    staleTime: 60_000,
  })
  const albumSearch = useQuery({
    queryKey: ['spotify-album-search', q, key],
    queryFn: () => searchSpotifyAlbums(q, key),
    enabled: mode === 'album' && q.length > 0,
    staleTime: 60_000,
  })
  const search = mode === 'artist' ? artistSearch : albumSearch

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ['artists'] })
    qc.invalidateQueries({ queryKey: ['albums'] })
    qc.invalidateQueries({ queryKey: ['spotify-search'] })
    qc.invalidateQueries({ queryKey: ['spotify-album-search'] })
  }

  const crawl = useMutation({
    mutationFn: (artistId: string) => crawlArtist(artistId, key),
    onSuccess: (res) => {
      setMessage(
        res.saved
          ? `✅ ${res.artist_name ?? '아티스트'}: 앨범 ${res.albums}개 저장 (크레딧 아티스트 ${res.artists ?? 0}명, 이미지 ${res.enriched ?? 0}개, 트랙 동기화 ${res.tracks_synced ?? 0}개)`
          : '앨범이 0개라 저장하지 않았습니다.',
      )
      invalidate()
    },
    onError: (e) => setMessage(`실패: ${String(e)}`),
  })

  const addAlbum = useMutation({
    mutationFn: (albumId: string) => crawlAlbum(albumId, key),
    onSuccess: (res) => {
      setMessage(
        `✅ ${res.album_name}: 저장 (크레딧 아티스트 ${res.artists ?? 0}명, 이미지 ${res.enriched ?? 0}개, 트랙 동기화 ${res.tracks_synced ?? 0}개)` +
          (res.deleted ? ' — 삭제된 앨범이라 목록에 보이지 않습니다. 「삭제된 앨범」 탭에서 복구하세요.' : ''),
      )
      invalidate()
    },
    onError: (e) => setMessage(`실패: ${String(e)}`),
  })

  const pending = crawl.isPending || addAlbum.isPending

  return (
    <div className={styles.aliasWrap}>
      <div className={styles.crawlControls}>
        <div className={styles.keyToggle} role="radiogroup" aria-label="검색 대상 선택">
          {(['artist', 'album'] as const).map((m) => (
            <button
              key={m}
              type="button"
              role="radio"
              aria-checked={mode === m}
              className={mode === m ? `${styles.tab} ${styles.active}` : styles.tab}
              onClick={() => {
                setMode(m)
                setMessage('')
              }}
            >
              {m === 'artist' ? '아티스트' : '앨범'}
            </button>
          ))}
        </div>

        {keys.length > 0 && (
          <div className={styles.keyToggle} role="radiogroup" aria-label="Spotify API 키 선택">
            {keys.map((k) => (
              <button
                key={k}
                type="button"
                role="radio"
                aria-checked={key === k}
                className={key === k ? `${styles.tab} ${styles.active}` : styles.tab}
                onClick={() => setPicked(k)}
              >
                {k.toUpperCase()}
              </button>
            ))}
          </div>
        )}

        <div className={styles.searchForm}>
          <Search size={15} className={styles.searchIcon} aria-hidden />
          <input
            className={styles.searchInput}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder={mode === 'artist' ? 'Spotify에서 아티스트 검색' : 'Spotify에서 앨범명 검색'}
            aria-label={mode === 'artist' ? 'Spotify 아티스트 검색' : 'Spotify 앨범 검색'}
          />
          {input && (
            <button type="button" className={styles.clearBtn} onClick={() => setInput('')} aria-label="검색어 지우기">
              <X size={14} />
            </button>
          )}
        </div>
      </div>

      {message && <p className={styles.crawlMsg}>{message}</p>}
      {search.error && <p className={styles.state}>검색 실패: {String(search.error)}</p>}
      {search.isFetching && <p className={styles.state}>검색 중…</p>}

      <div className={styles.rows}>
        {mode === 'artist' &&
          (artistSearch.data ?? []).map((hit) => (
            <div key={hit.id} className={styles.row}>
              <Avatar src={hit.image_url} name={hit.name} size={40} />
              <div className={styles.rowInfo}>
                <span className={styles.rowName} title={hit.name}>
                  {hit.name}
                </span>
                <span className={styles.rowId}>팔로워 {nf.format(hit.followers)}</span>
              </div>
              <span className={hit.albums_in_db > 0 ? `${styles.dbBadge} ${styles.dbBadgeHas}` : styles.dbBadge}>
                {hit.albums_in_db > 0 ? `DB ${hit.albums_in_db}개` : '미등록'}
              </span>
              <button
                type="button"
                className={styles.crawlBtn}
                disabled={pending}
                onClick={() => {
                  setMessage('')
                  crawl.mutate(hit.id)
                }}
              >
                <Download size={14} />
                {crawl.isPending && crawl.variables === hit.id ? '수집 중…' : '앨범 가져오기'}
              </button>
            </div>
          ))}

        {mode === 'album' &&
          (albumSearch.data ?? []).map((hit) => (
            <div key={hit.id} className={styles.row}>
              {hit.image_url ? (
                <img src={hit.image_url} alt="" className={styles.albumThumb} loading="lazy" />
              ) : (
                <div className={styles.albumThumb} aria-hidden />
              )}
              <div className={styles.rowInfo}>
                <span className={styles.rowName} title={hit.name}>
                  {hit.name}
                </span>
                <span className={styles.rowId}>
                  {[hit.artists, hit.release_date?.slice(0, 4), `${hit.total_tracks}곡`].filter(Boolean).join(' · ')}
                </span>
              </div>
              <span className={hit.in_db ? `${styles.dbBadge} ${styles.dbBadgeHas}` : styles.dbBadge}>
                {hit.deleted ? '삭제됨' : hit.in_db ? '등록됨' : '미등록'}
              </span>
              <button
                type="button"
                className={styles.crawlBtn}
                disabled={pending}
                onClick={() => {
                  setMessage('')
                  addAlbum.mutate(hit.id)
                }}
              >
                <Download size={14} />
                {addAlbum.isPending && addAlbum.variables === hit.id ? '추가 중…' : '앨범 추가'}
              </button>
            </div>
          ))}
      </div>
      {q && !search.isFetching && !search.error && (search.data?.length ?? 0) === 0 && (
        <p className={styles.state}>검색 결과가 없습니다.</p>
      )}
      {!q && (
        <p className={styles.state}>
          {mode === 'artist'
            ? '아티스트 이름으로 Spotify를 검색하세요.'
            : '앨범명으로 Spotify를 검색하세요. artist:이름, year:2024 같은 조건도 붙일 수 있습니다.'}
        </p>
      )}
    </div>
  )
}
