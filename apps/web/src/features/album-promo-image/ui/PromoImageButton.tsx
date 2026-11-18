import { useEffect, useRef, useState } from 'react'
import { Download, Flame, LoaderCircle, X } from 'lucide-react'
import { fetchAlbumTracks, type Album } from '@/entities/album'
import { promoFileName, renderPromoImages } from '../lib/renderPromoImage'
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

export function PromoImageButton({ album }: { album: Album }) {
  const [busy, setBusy] = useState(false)
  const [saving, setSaving] = useState(false)
  const [previews, setPreviews] = useState<Preview[] | null>(null)
  const dialog = useRef<HTMLDialogElement>(null)

  useEffect(() => {
    const el = dialog.current
    if (!el) return
    if (previews && !el.open) el.showModal()
    if (!previews && el.open) el.close()
  }, [previews])

  useEffect(() => () => previews?.forEach((p) => URL.revokeObjectURL(p.url)), [previews])

  const open = async () => {
    setBusy(true)
    try {
      const tracks = await fetchAlbumTracks(album.id)
      const blobs = await renderPromoImages(album, tracks)
      setPreviews(
        blobs.map((b, i) => {
          const file = new File([b], promoFileName(album, i), { type: 'image/png' })
          return { file, url: URL.createObjectURL(file) }
        }),
      )
    } catch (e) {
      alert(`홍보 이미지를 만들지 못했습니다: ${e instanceof Error ? e.message : String(e)}`)
    } finally {
      setBusy(false)
    }
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

  const close = () => setPreviews(null)

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
            <div className={styles.strip}>
              {previews.map((p, i) => (
                <img key={p.url} src={p.url} alt={`${i + 1}번째 이미지`} className={styles.preview} />
              ))}
            </div>
            <button type="button" className={styles.download} onClick={save} disabled={saving}>
              {saving ? <LoaderCircle size={16} className={styles.spin} /> : <Download size={16} strokeWidth={2.4} />}
              다운로드
            </button>
          </div>
        )}
      </dialog>
    </>
  )
}
