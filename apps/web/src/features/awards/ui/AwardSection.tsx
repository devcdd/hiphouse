import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { X } from 'lucide-react'
import { useAuth } from '@/entities/session'
import { AWARD_HOSTS, awardHostLabel } from '@/shared/lib/awardHosts'
import { addAward, fetchAwards, removeAward, type AwardKind } from '../api/awardApi'
import styles from './AwardSection.module.css'

// 주최는 정해진 시상식만 — 저장은 코드, 표시는 라벨. 목록에 없는 옛 값은 코드 그대로 보여준다.
const CUSTOM = '__custom__' // 수상명 select의 "직접 입력" 값 — 실제 수상명으로 저장되지 않는다

const EMPTY = { host: '', name: '', customName: '', year: '' }

// 수상 경력 — 누구나 보지만 추가/삭제는 관리자만. 항목이 없고 관리자도 아니면 렌더하지 않는다.
export function AwardSection({ kind, id }: { kind: AwardKind; id: string }) {
  const { isAdmin } = useAuth()
  const qc = useQueryClient()
  const queryKey = ['awards', kind, id]
  const { data: awards = [] } = useQuery({ queryKey, queryFn: () => fetchAwards(kind, id) })
  const [adding, setAdding] = useState(false)
  const [form, setForm] = useState(EMPTY)
  const set = (k: keyof typeof EMPTY) => (e: { target: { value: string } }) =>
    setForm((f) => ({ ...f, [k]: e.target.value }))
  const invalidate = () => {
    qc.invalidateQueries({ queryKey })
    // 카드 태그는 앨범 응답의 awards에서 나오므로 앨범 캐시도 같이
    if (kind === 'album') {
      qc.invalidateQueries({ queryKey: ['albums'] })
      qc.invalidateQueries({ queryKey: ['album', id] })
    }
  }
  const presets = AWARD_HOSTS.find((h) => h.code === form.host)?.awards ?? []
  const isCustom = presets.length === 0 || form.name === CUSTOM

  const add = useMutation({
    mutationFn: () =>
      addAward(kind, id, {
        host: form.host,
        name: (isCustom ? form.customName : form.name).trim(),
        year: form.year === '' ? null : Number(form.year),
      }),
    onSuccess: () => {
      setAdding(false)
      setForm(EMPTY)
      invalidate()
    },
  })
  const remove = useMutation({ mutationFn: removeAward, onSuccess: invalidate })

  if (awards.length === 0 && !isAdmin) return null

  return (
    <section className={styles.section} aria-label="수상 경력">
      <div className={styles.head}>
        수상 경력
        {awards.length > 0 && <b className={styles.count}>{awards.length}</b>}
        {isAdmin && !adding && (
          <button type="button" className={styles.addBtn} onClick={() => setAdding(true)}>
            + 추가
          </button>
        )}
      </div>

      {awards.length > 0 && (
        <ul className={styles.list}>
          {awards.map((a) => (
            <li key={a.id} className={styles.item}>
              <span className={styles.name}>{a.name}</span>
              <span className={styles.meta}>
                {awardHostLabel(a.host)}
                {a.year != null && (
                  <>
                    <i className={styles.dot}>·</i>
                    {a.year}
                  </>
                )}
              </span>
              {isAdmin && (
                <button
                  type="button"
                  className={styles.remove}
                  aria-label="수상 삭제"
                  disabled={remove.isPending}
                  onClick={() => remove.mutate(a.id)}
                >
                  <X size={12} strokeWidth={2.4} />
                </button>
              )}
            </li>
          ))}
        </ul>
      )}

      {adding && (
        <form
          className={styles.form}
          onSubmit={(e) => {
            e.preventDefault()
            add.mutate()
          }}
        >
          <select
            className={styles.input}
            value={form.host}
            // 주최가 바뀌면 프리셋 목록도 바뀌므로 수상명은 처음부터
            onChange={(e) => setForm((f) => ({ ...f, host: e.target.value, name: '', customName: '' }))}
            aria-label="주최"
            required
            autoFocus
          >
            <option value="" disabled>
              주최 선택
            </option>
            {AWARD_HOSTS.map((h) => (
              <option key={h.code} value={h.code} title={h.hint}>
                {h.label} ({h.code})
              </option>
            ))}
          </select>
          {presets.length > 0 && (
            <select className={styles.input} value={form.name} onChange={set('name')} aria-label="수상명" required>
              <option value="" disabled>
                수상명 선택
              </option>
              {presets.map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
              <option value={CUSTOM}>직접 입력</option>
            </select>
          )}
          {isCustom && (
            <input
              className={styles.input}
              value={form.customName}
              onChange={set('customName')}
              placeholder="수상명 직접 입력"
              aria-label="수상명 직접 입력"
              required
              autoFocus={presets.length > 0}
            />
          )}
          <input
            className={`${styles.input} ${styles.yearInput}`}
            type="number"
            min={1900}
            max={2100}
            value={form.year}
            onChange={set('year')}
            placeholder="연도"
            aria-label="연도"
            inputMode="numeric"
          />
          <button type="submit" className={styles.save} disabled={add.isPending}>
            {add.isPending ? '저장 중…' : '저장'}
          </button>
          <button type="button" className={styles.cancel} onClick={() => setAdding(false)}>
            취소
          </button>
          {add.isError && <span className={styles.error}>{add.error.message}</span>}
        </form>
      )}
    </section>
  )
}
