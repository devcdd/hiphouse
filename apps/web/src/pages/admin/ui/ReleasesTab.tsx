import { useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Clock, Disc3, Play, Square, Users } from 'lucide-react'
import { fetchSpotifyKeys } from '@/features/crawl-artist'
import { checkReleases, fetchReleasesStatus } from '../api/releasesAdminApi'
import styles from './AdminPage.module.css'

const nf = new Intl.NumberFormat('ko-KR')
const BATCH = 10 // 배치당 아티스트 수 — 서버가 아티스트당 Spotify 요청 1회
const LOG_MAX = 300

interface LogLine {
  kind: 'ok' | 'warn' | 'error' | 'info'
  text: string
}

// 신보 체크 탭: DB 보유 아티스트의 새 앨범을 배치로 감지해 자동 추가한다
// (2026-02 API 개편으로 사라진 new-releases의 로스터 기반 대체). 트랙 동기화
// 탭과 같은 실행 계약 — 남은 아티스트가 0이 되거나, 중지를 누르거나, Spotify
// 오류가 나면 멈춘다. 20시간 주기라 매일 한 번 돌리면 어제 확인분부터 다시 돈다.
export function ReleasesTab() {
  const qc = useQueryClient()
  const status = useQuery({ queryKey: ['releases-status'], queryFn: fetchReleasesStatus })
  const keysQuery = useQuery({ queryKey: ['spotify-keys'], queryFn: fetchSpotifyKeys, staleTime: Infinity })
  const keys = keysQuery.data ?? []
  const [picked, setPicked] = useState('')
  const key = picked && keys.includes(picked) ? picked : (keys[0] ?? '') // '' = server default pair

  const [running, setRunning] = useState(false)
  const [stopping, setStopping] = useState(false)
  const [log, setLog] = useState<LogLine[]>([])
  const stopRef = useRef(false)
  // 탭을 벗어나면 다음 배치로 넘어가지 않는다 (진행 중인 배치는 서버가 마저 저장).
  useEffect(
    () => () => {
      stopRef.current = true
    },
    [],
  )

  const pushLog = (lines: LogLine[]) => setLog((prev) => [...lines, ...prev].slice(0, LOG_MAX))

  async function run() {
    stopRef.current = false
    setRunning(true)
    setStopping(false)
    setLog([])
    let checked = 0
    let albums = 0
    try {
      for (;;) {
        const res = await checkReleases(key, BATCH)
        checked += res.checked
        albums += res.new_albums
        if (res.artists.length > 0) {
          // 로그는 최신이 위: 배치 내 처리 순서를 뒤집어 마지막 아티스트가 맨 위로.
          pushLog(
            res.artists
              .map<LogLine>((a) => ({
                kind: 'ok',
                text: `${a.name ?? a.id} — 새 앨범 ${a.albums.length}개: ${a.albums.join(', ')}`,
              }))
              .reverse(),
          )
          qc.invalidateQueries({ queryKey: ['albums'] })
          qc.invalidateQueries({ queryKey: ['artists'] })
        } else if (res.checked > 0) {
          pushLog([{ kind: 'info', text: `${res.checked}명 확인 — 신보 없음` }])
        }
        qc.invalidateQueries({ queryKey: ['releases-status'] })
        if (res.error) {
          pushLog([{ kind: 'error', text: `중단됨: ${res.error}` }])
          break
        }
        if (res.remaining === 0) {
          pushLog([
            { kind: 'info', text: `✅ 완료 — 아티스트 ${nf.format(checked)}명 확인, 새 앨범 ${nf.format(albums)}개` },
          ])
          break
        }
        if (stopRef.current) {
          pushLog([
            {
              kind: 'info',
              text: `중지 — ${nf.format(checked)}명 확인, 새 앨범 ${nf.format(albums)}개, ${nf.format(res.remaining)}명 남음`,
            },
          ])
          break
        }
        if (res.checked === 0) {
          pushLog([{ kind: 'error', text: '진행되지 않아 중단했습니다.' }]) // 무한 루프 방지
          break
        }
      }
    } catch (e) {
      pushLog([{ kind: 'error', text: `요청 실패: ${String(e)}` }])
    } finally {
      setRunning(false)
      qc.invalidateQueries({ queryKey: ['releases-status'] })
    }
  }

  const s = status.data
  const cards = s
    ? [
        { label: '대상 아티스트', value: s.artists, Icon: Users },
        { label: '확인 대기', value: s.stale, Icon: Clock },
      ]
    : []

  return (
    <div className={styles.aliasWrap}>
      {status.isLoading ? (
        <p className={styles.state}>불러오는 중…</p>
      ) : status.error || !s ? (
        <p className={styles.state}>불러오기 실패: {String(status.error)}</p>
      ) : (
        <div className={styles.statGrid}>
          {cards.map(({ label, value, Icon }) => (
            <div key={label} className={styles.statCard}>
              <Icon size={16} className={styles.statIcon} aria-hidden />
              <span className={styles.statLabel}>{label}</span>
              <strong className={styles.statValue}>{nf.format(value)}</strong>
            </div>
          ))}
        </div>
      )}

      <div className={styles.crawlControls}>
        {keys.length > 0 && (
          <div className={styles.keyToggle} role="radiogroup" aria-label="Spotify API 키 선택">
            {keys.map((k) => (
              <button
                key={k}
                type="button"
                role="radio"
                aria-checked={key === k}
                disabled={running}
                className={key === k ? `${styles.tab} ${styles.active}` : styles.tab}
                onClick={() => setPicked(k)}
              >
                {k.toUpperCase()}
              </button>
            ))}
          </div>
        )}

        {running ? (
          <button
            type="button"
            className={styles.crawlBtn}
            disabled={stopping}
            onClick={() => {
              stopRef.current = true
              setStopping(true)
            }}
          >
            <Square size={14} />
            {stopping ? '중지 중…' : '중지'}
          </button>
        ) : (
          <button type="button" className={styles.crawlBtn} disabled={!s || s.stale === 0} onClick={run}>
            <Play size={14} />
            신보 체크{s && s.stale > 0 ? ` (${nf.format(s.stale)}명)` : ''}
          </button>
        )}
        {running && (
          <span className={styles.runNote}>새 앨범은 발견 즉시 저장되며, 중지해도 완료된 아티스트는 유지됩니다.</span>
        )}
      </div>

      {s && s.stale === 0 && !running && log.length === 0 && (
        <p className={styles.state}>이번 주기의 모든 아티스트를 확인했습니다. 20시간 후 다시 대상이 됩니다.</p>
      )}

      {log.length > 0 && (
        <div className={styles.trackLog} role="log" aria-live="polite">
          {log.map((l, i) => (
            <p
              key={log.length - i}
              className={
                l.kind === 'error'
                  ? styles.trackLogErr
                  : l.kind === 'warn'
                    ? styles.trackLogWarn
                    : l.kind === 'info'
                      ? styles.trackLogInfo
                      : styles.trackLogLine
              }
            >
              {l.text}
            </p>
          ))}
        </div>
      )}
      <p className={styles.state}>
        <Disc3 size={13} aria-hidden style={{ verticalAlign: '-2px', marginRight: 4 }} />
        신보 감시가 켜진 아티스트만 확인합니다 (아티스트 상세의 「신보 감시」 버튼으로 관리). 아티스트당 Spotify
        앨범 목록 첫 페이지(최신 10장)를 비교해 DB에 없는 앨범만 추가하며, 삭제한 앨범은 되살리지 않습니다.
      </p>
    </div>
  )
}
