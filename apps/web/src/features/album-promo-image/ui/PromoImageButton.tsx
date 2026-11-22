import { useEffect, useRef, useState } from 'react'
import { Dices, Download, Flame, LoaderCircle, X } from 'lucide-react'
import { fetchAlbumTracks, type Album, type Track } from '@/entities/album'
import { PROMO_BACKGROUNDS, promoFileName, renderPromoImages, type PromoOptions } from '../lib/renderPromoImage'
import { FILM_STYLES, randomFilmSeed, type FilmStyle } from '../lib/shrinkWrap'
import styles from './PromoImageButton.module.css'

// 휴대폰에선 공유 시트로 여러 장을 한 번에 넘겨 인스타그램 캐러셀로 바로 올리고, 데스크톱에선 파일로 내려받는다.
async function deliver(files: File[]) {
  const mobile = window.matchMedia('(pointer: coarse)').matches
  if (mobile && navigator.canShare?.({ files })) {
    try {
      await navigator.share({ files })
      return
    } catch (e) {
      if (e instanceof DOMException && e.name === 'AbortError') return
    }
  }
  for (const file of files) {
    const url = URL.createObjectURL(file)
    const a = document.createElement('a')
    a.href = url
    a.download = file.name
    a.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
    // 연달아 클릭하면 브라우저가 첫 파일만 받는 경우가 있어서 사이를 둔다.
    await new Promise((r) => setTimeout(r, 300))
  }
}

interface Preview {
  file: File
  url: string
}

export function PromoImageButton({ album, options }: { album: Album; options?: PromoOptions }) {
  const [busy, setBusy] = useState(false)
  const [saving, setSaving] = useState(false)
  const [previews, setPreviews] = useState<Preview[] | null>(null)
  // null이면 커버에서 뽑은 색을 쓴다.
  const [background, setBackground] = useState<string | null>(null)
  // null이면 시드가 질감을 고른다.
  const [film, setFilm] = useState<FilmStyle | null>(null)
  // 배경색이나 질감만 바꿀 때는 주름이 그대로여야 비교할 수 있어서, 열 때와 다시 뽑을 때만 새로 정한다.
  const filmSeed = useRef(0)
  const [rendering, setRendering] = useState(false)
  const dialog = useRef<HTMLDialogElement>(null)
  const tracks = useRef<Track[] | null>(null)
  // 옵션을 연달아 바꾸면 늦게 끝난 이전 렌더가 화면을 덮어쓰지 않게 마지막 요청만 반영한다.
  const latest = useRef(0)

  useEffect(() => {
    const el = dialog.current
    if (!el) return
    if (previews && !el.open) el.showModal()
    if (!previews && el.open) el.close()
  }, [previews])

  useEffect(() => () => previews?.forEach((p) => URL.revokeObjectURL(p.url)), [previews])

  const render = async (bgId: string | null, filmStyle: FilmStyle | null) => {
    const req = ++latest.current
    tracks.current ??= await fetchAlbumTracks(album.id)
    const blobs = await renderPromoImages(album, tracks.current, {
      ...options,
      background: PROMO_BACKGROUNDS.find((b) => b.id === bgId)?.rgb,
      film: filmStyle ?? undefined,
      filmSeed: filmSeed.current,
    })
    if (req !== latest.current) return
    setPreviews(
      blobs.map((b, i) => {
        const file = new File([b], promoFileName(album, i, options), { type: 'image/png' })
        return { file, url: URL.createObjectURL(file) }
      }),
    )
  }

  const fail = (e: unknown) => alert(`홍보 이미지를 만들지 못했습니다: ${e instanceof Error ? e.message : String(e)}`)

  const open = async () => {
    setBusy(true)
    filmSeed.current = randomFilmSeed()
    try {
      await render(background, film)
    } catch (e) {
      fail(e)
    } finally {
      setBusy(false)
    }
  }

  const redraw = async (bgId: string | null, filmStyle: FilmStyle | null) => {
    setRendering(true)
    try {
      await render(bgId, filmStyle)
    } catch (e) {
      fail(e)
    } finally {
      setRendering(false)
    }
  }

  const pick = (bgId: string | null) => {
    if (bgId === background) return
    setBackground(bgId)
    redraw(bgId, film)
  }

  const pickFilm = (filmStyle: FilmStyle | null) => {
    if (filmStyle === film) return
    setFilm(filmStyle)
    redraw(background, filmStyle)
  }

  const reroll = () => {
    filmSeed.current = randomFilmSeed()
    redraw(background, film)
  }

  const save = async () => {
    if (!previews) return
    setSaving(true)
    try {
      await deliver(previews.map((p) => p.file))
    } finally {
      setSaving(false)
    }
  }

  const close = () => {
    latest.current++
    setPreviews(null)
    setRendering(false)
  }

  return (
    <>
      <button
        type="button"
        className={styles.btn}
        onClick={open}
        disabled={busy || !album.image_url}
        aria-label="홍보 이미지 미리보기"
        title="홍보 이미지 미리보기"
      >
        {busy ? <LoaderCircle size={16} className={styles.spin} /> : <Flame size={16} strokeWidth={2.2} />}
      </button>

      <dialog
        ref={dialog}
        className={styles.dialog}
        onClose={close}
        onClick={(e) => e.target === dialog.current && close()}
      >
        {previews && (
          <div className={styles.sheet}>
            <div className={styles.head}>
              <p className={styles.title}>홍보 이미지 {previews.length}장</p>
              <button type="button" className={styles.close} onClick={close} aria-label="닫기">
                <X size={18} strokeWidth={2.4} />
              </button>
            </div>
            <div className={styles.swatches} role="radiogroup" aria-label="배경색">
              <span className={styles.rowLabel}>배경</span>
              <button
                type="button"
                role="radio"
                aria-checked={background === null}
                className={`${styles.swatch} ${styles.auto}`}
                onClick={() => pick(null)}
              >
                자동
              </button>
              {PROMO_BACKGROUNDS.map((b) => (
                <button
                  key={b.id}
                  type="button"
                  role="radio"
                  aria-checked={background === b.id}
                  aria-label={b.name}
                  title={b.name}
                  className={styles.swatch}
                  style={{ background: `rgb(${b.rgb.join(',')})` }}
                  onClick={() => pick(b.id)}
                />
              ))}
              {rendering && <LoaderCircle size={16} className={styles.spin} />}
            </div>
            <div className={styles.swatches}>
              <span className={styles.rowLabel}>비닐</span>
              <div className={styles.films} role="radiogroup" aria-label="비닐 질감">
                <button
                  type="button"
                  role="radio"
                  aria-checked={film === null}
                  className={`${styles.swatch} ${styles.auto}`}
                  onClick={() => pickFilm(null)}
                >
                  랜덤
                </button>
                {FILM_STYLES.map((f) => (
                  <button
                    key={f.id}
                    type="button"
                    role="radio"
                    aria-checked={film === f.id}
                    className={`${styles.swatch} ${styles.auto}`}
                    onClick={() => pickFilm(f.id)}
                  >
                    {f.name}
                  </button>
                ))}
              </div>
              <button
                type="button"
                className={styles.reroll}
                onClick={reroll}
                disabled={rendering}
                aria-label="비닐 다시 뽑기"
                title="비닐 다시 뽑기"
              >
                <Dices size={18} strokeWidth={2.2} />
              </button>
            </div>
            <div className={styles.strip} data-rendering={rendering}>
              {previews.map((p, i) => (
                <img key={p.url} src={p.url} alt={`${i + 1}번째 이미지`} className={styles.preview} />
              ))}
            </div>
            <button type="button" className={styles.download} onClick={save} disabled={saving || rendering}>
              {saving ? <LoaderCircle size={16} className={styles.spin} /> : <Download size={16} strokeWidth={2.4} />}
              다운로드
            </button>
          </div>
        )}
      </dialog>
    </>
  )
}
