export interface User {
  id: string
  nickname: string
  role: string
  profile_public: boolean // false = 남이 보는 내 공개 프로필에서 활동을 전부 가린다 (본인에게는 계속 보인다)
  /** 본인이 닉네임을 저장한 적 있는지. false면 첫 로그인 인사 + 닉네임 설정 화면으로 보낸다. */
  nickname_set: boolean
  /** API 기준 프로필 이미지 경로. `apiUrl()`을 거쳐 `<img src>`에 넣는다. 이미지가 없으면 null. */
  avatar_url: string | null
  /** 탈퇴 신청 뒤 영구 삭제될 시각(ISO 8601). 신청하지 않았으면 null. */
  delete_after: string | null
}
