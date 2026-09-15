import { useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Disc3, Link2, Play, Square, Users } from 'lucide-react'
import { fetchAppleStatus, linkApple } from '../api/appleAdminApi'
import styles from './AdminPage.module.css'

const nf = new Intl.NumberFormat('ko-KR')
const BATCH = 50
const LOG_MAX = 300

interface LogLine {
  kind: 'ok' | 'error' | 'info'
  text: string
}

// Apple 연동 탭: UPC(없으면 검색)로 Apple 앨범을 찾아 레이블·장르·apple_id와
// 비어 있는 한글 표시 이름을 백필한다. 새 앨범은 크롤/신보 체크가, 백로그는 서버
// 스윕이 자동 처리하므로 이 탭은 즉시 돌리고 싶을 때용. 트랙 동기화와 같은 실행 계약 — 남은 앨범 0, 중지, 오류 시 멈춘다.
export function AppleTab() {
  const qc = useQueryClient()
  const status = useQuery({ queryKey: ['apple-status'], queryFn: fetchAppleStatus })
  const [running, setRunning] = useState(false)
  const [stopping, setStopping] = useState(false)
  const [log, setLog] = useState<LogLine[]>([])
  const stopRef = useRef(false)
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
    let linked = 0
    let artists = 0
    let named = 0
    try {
      for (;;) {
        const res = await linkApple(BATCH)
        checked += res.checked
        linked += res.linked
        artists += res.artists_linked
        named += res.named
        if (res.checked > 0) {
          pushLog([
            {
              kind: res.linked > 0 ? 'ok' : 'info',
              text: `${res.checked}개 조회 — 앨범 ${res.linked}개, 아티스트 ${res.artists_linked}명 연결, 한글 이름 ${res.named}개`,
            },
          ])
          qc.invalidateQueries({ queryKey: ['albums'] })
          qc.invalidateQueries({ queryKey: ['artists'] })
        }
        qc.invalidateQueries({ queryKey: ['apple-status'] })
        if (res.error) {
          pushLog([{ kind: 'error', text: `중단됨: ${res.error}` }])
          break
        }
        if (res.remaining === 0) {
          pushLog([
            {
              kind: 'info',
              text: `✅ 완료 — ${nf.format(checked)}개 조회, 앨범 ${nf.format(linked)}개·아티스트 ${nf.format(artists)}명 연결, 한글 이름 ${nf.format(named)}개`,
            },
          ])
          break
        }
        if (stopRef.current) {
          pushLog([{ kind: 'info', text: `중지 — ${nf.format(checked)}개 조회, ${nf.format(res.remaining)}개 남음` }])
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
      qc.invalidateQueries({ queryKey: ['apple-status'] })
    }
  }

  const s = status.data
  const cards = s
    ? [
        { label: '연결된 앨범', value: `${nf.format(s.linked)} / ${nf.format(s.albums)}`, Icon: Link2 },
        { label: '조회 대기', value: nf.format(s.pending), Icon: Disc3 },
        { label: '연결된 아티스트', value: nf.format(s.artists_linked), Icon: Users },
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
              <strong className={styles.statValue}>{value}</strong>
            </div>
          ))}
        </div>
      )}

      {s && !s.configured && (
        <p className={styles.state}>서버에 APPLE_MUSIC_* 환경변수가 없습니다. .env.example을 참고해 설정하세요.</p>
      )}

      <div className={styles.crawlControls}>
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
          <button
            type="button"
            className={styles.crawlBtn}
            disabled={!s || !s.configured || s.pending === 0}
            onClick={run}
          >
            <Play size={14} />
            Apple 연동{s && s.pending > 0 ? ` (${nf.format(s.pending)}개)` : ''}
          </button>
        )}
        {running && <span className={styles.runNote}>연결 결과는 즉시 저장되며, 중지해도 완료된 앨범은 유지됩니다.</span>}
      </div>

      {log.length > 0 && (
        <div className={styles.trackLog} role="log" aria-live="polite">
          {log.map((l, i) => (
            <p
              key={log.length - i}
              className={
                l.kind === 'error' ? styles.trackLogErr : l.kind === 'info' ? styles.trackLogInfo : styles.trackLogLine
              }
            >
              {l.text}
            </p>
          ))}
        </div>
      )}
      <p className={styles.state}>
        UPC(없으면 아티스트+앨범명 검색)로 Apple Music 카탈로그를 조회해 앨범의 레이블·장르, 크레딧 아티스트의 장르를 채우고,
        비어 있는 한글 표시 이름(앨범·아티스트·트랙)을 Apple 한글 표기로 채웁니다. 관리자가 넣은 표시 이름은 건드리지 않습니다.
        새 앨범은 크롤·신보 체크가 자동 연결하고, 백로그는 이틀 주기 서버 스윕이 마저 처리합니다. Apple에 없는 앨범은 한 번 조회 후
        건너뜁니다.
      </p>
    </div>
  )
}
