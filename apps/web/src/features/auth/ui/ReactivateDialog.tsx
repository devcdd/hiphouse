import { useEffect, useRef } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { cancelWithdraw, formatDeleteAfter, useAuth } from '@/entities/session'
import { useToast } from '@/shared/ui/toast'
import styles from './ReactivateDialog.module.css'

// 탈퇴 유예 기간 안에 다시 로그인하면 어느 화면에서든 뜬다. 고를 때까지 닫히지 않는다.
// "아니요"는 로그아웃만 하고 삭제 예정은 그대로 둔다.
export function ReactivateDialog() {
  const { user, signOut } = useAuth()
  const qc = useQueryClient()
  const toast = useToast()
  const dialog = useRef<HTMLDialogElement>(null)
  const deleteAfter = user?.delete_after ?? null

  const reactivate = useMutation({
    mutationFn: cancelWithdraw,
    onSuccess: (u) => {
      qc.setQueryData(['me'], u)
      toast('계정을 다시 활성화했습니다')
    },
    onError: (e: Error) => toast(e.message || '계정 활성화에 실패했습니다', 'error'),
  })

  useEffect(() => {
    const el = dialog.current
    if (!el) return
    if (deleteAfter && !el.open) el.showModal()
    if (!deleteAfter && el.open) el.close()
  }, [deleteAfter])

  return (
    <dialog
      ref={dialog}
      className={styles.dialog}
      aria-labelledby="reactivate-title"
      // Esc로 닫히면 탈퇴 예정 계정으로 사이트를 계속 쓰게 되니 막는다.
      onCancel={(e) => e.preventDefault()}
    >
      {deleteAfter && (
        <div className={styles.sheet}>
          <h2 id="reactivate-title" className={styles.title}>
            탈퇴 신청된 계정입니다
          </h2>
          <p className={styles.body}>
            {formatDeleteAfter(deleteAfter)}에 계정과 활동 기록이 영구 삭제될 예정입니다. 다시 활성화하시겠습니까?
          </p>
          <div className={styles.actions}>
            <button type="button" className={styles.secondary} disabled={reactivate.isPending} onClick={signOut}>
              아니요
            </button>
            <button
              type="button"
              className={styles.primary}
              disabled={reactivate.isPending}
              onClick={() => reactivate.mutate()}
            >
              {reactivate.isPending ? '활성화 중…' : '다시 활성화'}
            </button>
          </div>
        </div>
      )}
    </dialog>
  )
}
