export interface User {
  id: string
  nickname: string
  role: string
  /** 본인이 닉네임을 저장한 적 있는지. false면 첫 로그인 인사 + 닉네임 설정 화면으로 보낸다. */
  nickname_set: boolean
}
