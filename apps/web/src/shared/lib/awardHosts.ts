// 수상 주최 프리셋 — 저장은 코드, 표시는 라벨. awards가 비면 수상명은 직접 입력만.
export const AWARD_HOSTS = [
  {
    code: 'KMA',
    label: '한국대중음악상',
    hint: '평론가·관계자 선정, 최우수 랩&힙합 음반/노래',
    awards: ['최우수 랩&힙합 음반', '올해의 음반'],
  },
  {
    code: 'KHA',
    label: '한국 힙합 어워즈',
    hint: '힙합플레이야·힙합엘이 공동 주최',
    awards: [
      '올해의 아티스트',
      '올해의 신인 아티스트',
      '올해의 힙합 앨범',
      '올해의 힙합 트랙',
      '올해의 알앤비 앨범',
      '올해의 알앤비 트랙',
      '올해의 프로듀서',
      '올해의 콜라보레이션',
      '올해의 뮤직비디오',
      '올해의 레이블',
    ],
  },
  { code: 'MAMA', label: 'MAMA Awards', hint: 'Best Rap & Hip Hop Performance 등', awards: [] },
  { code: 'MMA', label: 'Melon Music Awards', hint: '음원 성적 비중 큼', awards: [] },
  { code: 'GDA', label: 'Golden Disc Awards', hint: '음반·음원 성적 중심', awards: [] },
]

// 목록에 없는 옛 값은 코드 그대로.
export const awardHostLabel = (code: string) => AWARD_HOSTS.find((h) => h.code === code)?.label ?? code
