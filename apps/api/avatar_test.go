package main

import (
	"bytes"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

func encodeTestImage(t *testing.T, w, h int, enc func(*bytes.Buffer, image.Image) error) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := enc(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func pngEnc(b *bytes.Buffer, m image.Image) error  { return png.Encode(b, m) }
func jpegEnc(b *bytes.Buffer, m image.Image) error { return jpeg.Encode(b, m, nil) }
func gifEnc(b *bytes.Buffer, m image.Image) error  { return gif.Encode(b, m, nil) }

// RIFF/WEBP 헤더와 VP8L 비트스트림 1×1 투명 픽셀. x/image에는 WebP 인코더가 없어 바이트를 박아 둔다.
var tinyWebP = []byte{
	0x52, 0x49, 0x46, 0x46, 0x1a, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50,
	0x56, 0x50, 0x38, 0x4c, 0x0d, 0x00, 0x00, 0x00, 0x2f, 0x00, 0x00, 0x00,
	0x10, 0x07, 0x10, 0x11, 0x11, 0x88, 0x88, 0xfe, 0x07, 0x00,
}

func TestValidateAvatar(t *testing.T) {
	cases := []struct {
		name     string
		data     []byte
		wantType string
	}{
		{"png", encodeTestImage(t, 256, 256, pngEnc), "image/png"},
		{"jpeg", encodeTestImage(t, 256, 256, jpegEnc), "image/jpeg"},
		{"webp", tinyWebP, "image/webp"},
		{"최대 크기", encodeTestImage(t, avatarMaxSide, avatarMaxSide, pngEnc), "image/png"},
		{"가로 초과", encodeTestImage(t, avatarMaxSide+1, 10, pngEnc), ""},
		{"gif는 거부", encodeTestImage(t, 10, 10, gifEnc), ""},
		{"이미지 아님", []byte("<svg xmlns='http://www.w3.org/2000/svg'/>"), ""},
		{"빈 본문", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := validateAvatar(c.data)
			if c.wantType == "" {
				if err == nil {
					t.Fatalf("got %q, want error", got)
				}
				return
			}
			if err != nil || got != c.wantType {
				t.Fatalf("got (%q, %v), want %q", got, err, c.wantType)
			}
		})
	}
}
