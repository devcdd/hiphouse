package main

import (
	"strings"
	"testing"
)

func TestValidWallBodyCountsRunes(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		invalid bool
	}{
		{in: "  안녕  ", want: "안녕"},
		{in: "   ", invalid: true},
		{in: strings.Repeat("가", 50), want: strings.Repeat("가", 50)}, // 한글 50자는 바이트로 150이라도 통과
		{in: strings.Repeat("가", 51), invalid: true},
	}
	for _, c := range cases {
		got, msg := validWallBody(c.in)
		if (msg != "") != c.invalid || got != c.want {
			t.Errorf("validWallBody(%q) = %q, %q", c.in, got, msg)
		}
	}
}
