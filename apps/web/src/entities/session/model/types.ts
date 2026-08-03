export interface User {
  id: string
  nickname: string
  role: string
  profile_public: boolean // false = 남이 보는 내 공개 프로필에서 활동을 전부 가린다 (본인에게는 계속 보인다)
  /** 본인이 닉네임을 저장한 적 있는지. false면 첫 로그인 인사 + 닉네임 설정 화면으로 보낸다. */
  nickname_set: boolean
}
