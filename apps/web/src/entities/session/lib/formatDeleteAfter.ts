const fmt = new Intl.DateTimeFormat('ko-KR', { month: 'long', day: 'numeric', weekday: 'short' })

// 탈퇴 안내 문구용: "11월 29일 (일)"
export const formatDeleteAfter = (iso: string) => fmt.format(new Date(iso))
