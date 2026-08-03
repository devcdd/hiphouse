export interface User {
  id: string
  nickname: string
  role: string
  profile_public: boolean // false = 공개 프로필의 내 활동을 전부 가린다 (/me/privacy)
}
