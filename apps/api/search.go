package main

// chosung은 초성 열아홉 자를 유니코드 한글 음절이 놓인 순서대로 적은 것이다.
var chosung = []rune("ㄱㄲㄴㄷㄸㄹㅁㅂㅃㅅㅆㅇㅈㅉㅊㅋㅌㅍㅎ")

// 한글 음절은 U+AC00부터 초성마다 588자씩(중성 21 × 종성 28) 이어진다.
const (
	hangulBase      = 0xAC00
	syllablesPerCho = 588
)

// chosungClass는 초성 자모를 그 초성으로 시작하는 모든 음절과 맞는 문자 클래스로 바꾼다.
// "ㅂㅈㄴ"으로 "빈지노"를 찾는 초성 검색에 쓴다. 자모를 그대로 쓴 이름("ㅋㅋ")도 맞도록 자모 자신도 넣는다.
func chosungClass(r rune) (string, bool) {
	for i, c := range chosung {
		if c == r {
			first := rune(hangulBase + i*syllablesPerCho)
			return "[" + string(r) + string(first) + "-" + string(first+syllablesPerCho-1) + "]", true
		}
	}
	return "", false
}
