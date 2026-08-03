export interface User {
  id: string
  nickname: string
  role: string
  profile_public: boolean // false = 공개 프로필의 내 활동을 전부 가린다 (/me/privacy)
  /** 본인이 닉네임을 저장한 적 있는지. false면 첫 로그인 인사 + 닉네임 설정 화면으로 보낸다. */
  nickname_set: boolean
}
