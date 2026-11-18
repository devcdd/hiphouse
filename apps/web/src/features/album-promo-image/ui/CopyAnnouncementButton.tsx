import { useState } from 'react'
import { LoaderCircle, MessageSquareText } from 'lucide-react'
import { fetchAlbumTracks, type Album } from '@/entities/album'
import { useToast } from '@/shared/ui/toast'
import { announcementText } from '../lib/announcement'
import styles from './PromoImageButton.module.css'

// Safari는 클릭 뒤 await를 거치면 사용자 동작이 끊긴 것으로 보고 클립보드 쓰기를 막는다.
// ClipboardItem에 Promise를 넘기면 클릭 순간에 쓰기를 시작하고 내용은 나중에 채울 수 있다.
async function copyText(text: Promise<string>) {
  if (typeof ClipboardItem !== 'undefined' && navigator.clipboard.write) {
    const blob = text.then((t) => new Blob([t], { type: 'text/plain' }))
    await navigator.clipboard.write([new ClipboardItem({ 'text/plain': blob })])
    return
  }
  await navigator.clipboard.writeText(await text)
}

export function CopyAnnouncementButton({ album }: { album: Album }) {
  const toast = useToast()
  const [busy, setBusy] = useState(false)

  const onClick = async () => {
    setBusy(true)
    try {
      const text = fetchAlbumTracks(album.id).then((tracks) => announcementText(album, tracks))
      await copyText(text)
      toast('홍보 멘트를 복사했습니다')
    } catch {
      toast('홍보 멘트를 복사하지 못했습니다', 'error')
    } finally {
      setBusy(false)
    }
  }

  return (
    <button
      type="button"
      className={styles.btn}
      onClick={onClick}
      disabled={busy}
      aria-label="홍보 멘트 복사"
      title="홍보 멘트 복사"
    >
      {busy ? <LoaderCircle size={16} className={styles.spin} /> : <MessageSquareText size={16} strokeWidth={2.2} />}
    </button>
  )
}
