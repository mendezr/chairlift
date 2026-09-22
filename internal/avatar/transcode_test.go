package avatar

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// readWebPSample loads one of the package's frozen WebP payloads from
// testdata/, relative to the package directory the test binary runs in.
func readWebPSample(t *testing.T, name string) []byte {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading testdata sample %s: %v", name, err)
	}
	return payload
}

// TestTranscodeWebPToPNGScalesSamplePayloadsToSquarePNGWithinCeiling pins the
// happy path across the three sample WebP payloads: each decodes, comes back
// as a valid 512x512 PNG, and fits under the issue's 1 MiB ceiling. The
// fourcc assertion proves each case still carries the format it claims —
// VP8L (the issue's named codec), extended VP8X with alpha, and bare lossy
// VP8 — so a fixture swap cannot silently turn the table into three copies
// of one format.
func TestTranscodeWebPToPNGScalesSamplePayloadsToSquarePNGWithinCeiling(t *testing.T) {
	const oneMebibyte = 1_048_576 // the issue's ceiling, pinned independently of MaxPNGBytes
	if MaxPNGBytes != oneMebibyte {
		t.Fatalf("MaxPNGBytes = %d, want %d", MaxPNGBytes, oneMebibyte)
	}
	cases := []struct {
		file   string
		fourcc string
	}{
		{"katharina.webp", "VP8L"},      // real catalog artwork (characters/header/katharina.webp), lossless
		{"dakota.webp", "VP8X"},         // real catalog artwork (characters/dakota.webp), extended with alpha
		{"gradient-lossy.webp", "VP8 "}, // synthetic lossy sample
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			payload := readWebPSample(t, tc.file)
			if len(payload) < 16 {
				t.Fatalf("sample is %d bytes, too short to carry a RIFF fourcc", len(payload))
			}
			if got := string(payload[12:16]); got != tc.fourcc {
				t.Fatalf("fourcc at byte 12 = %q, want %q; the sample no longer exercises the format this case names", got, tc.fourcc)
			}
			out, err := TranscodeWebPToPNG(bytes.NewReader(payload))
			if err != nil {
				t.Fatalf("TranscodeWebPToPNG() error = %v", err)
			}
			if len(out) > oneMebibyte {
				t.Errorf("encoded PNG is %d bytes, over the %d-byte ceiling", len(out), oneMebibyte)
			}
			img, err := png.Decode(bytes.NewReader(out))
			if err != nil {
				t.Fatalf("decoding the result as PNG: %v", err)
			}
			if want := image.Rect(0, 0, AvatarSize, AvatarSize); img.Bounds() != want {
				t.Errorf("bounds = %v, want %v", img.Bounds(), want)
			}
		})
	}
}

// TestTranscodeWebPToPNGRejectsPayloadsThatDoNotDecode covers the decode
// failure outcome: no bytes come back, the error is a decode failure rather
// than the ceiling sentinel, and each shape is one a fetch of the catalog's
// artwork could actually produce (empty body, non-WebP content, a RIFF
// wrapper around garbage, a truncated read).
func TestTranscodeWebPToPNGRejectsPayloadsThatDoNotDecode(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
	}{
		{"empty", nil},
		{"not a webp", []byte("this is not a WebP image")},
		{"riff header with junk body", []byte("RIFF\x10\x00\x00\x00WEBPVP8L junkjunkjunkjunk")},
		{"truncated sample", readWebPSample(t, "katharina.webp")[:64]},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := TranscodeWebPToPNG(bytes.NewReader(tc.payload))
			if err == nil {
				t.Fatal("TranscodeWebPToPNG() error = nil, want a decode failure")
			}
			if errors.Is(err, ErrAvatarTooLarge) {
				t.Errorf("error = %v; a payload that does not decode is a decode failure, not a ceiling failure", err)
			}
			if out != nil {
				t.Errorf("bytes = %d long, want nil on failure", len(out))
			}
		})
	}
}

// TestTranscodeWebPToPNGReportsTooLargeWhenNoRungFitsTheCeiling drives the
// explicit ErrAvatarTooLarge outcome. A ceiling of zero can never be met, so
// the whole 512 -> 384 -> 256 ladder is attempted before the sentinel comes
// back — this is the reduction-failed path the issue requires, and the seam
// that keeps it testable without a multi-megabyte fixture.
func TestTranscodeWebPToPNGReportsTooLargeWhenNoRungFitsTheCeiling(t *testing.T) {
	payload := readWebPSample(t, "gradient-lossy.webp")
	out, err := transcodeWebPToPNG(bytes.NewReader(payload), 0)
	if !errors.Is(err, ErrAvatarTooLarge) {
		t.Fatalf("error = %v, want ErrAvatarTooLarge", err)
	}
	if out != nil {
		t.Errorf("bytes = %d long, want nil when every rung exceeds the ceiling", len(out))
	}
}

// TestTranscodeWebPToPNGDownsamplesWhenTheFullSizeEncodeExceedsTheCeiling
// proves the ladder actually downsamples instead of failing at full size: a
// ceiling below the full-size attempt but above the 384 rung must come back
// as a valid 384x384 PNG within budget.
//
// downsampleCeiling was measured through transcodeWebPToPNG itself on
// go1.26.6 with png.BestCompression: the katharina.webp full-size attempt
// encodes to 236,281 bytes and the 384 rung to 144,264, so 184,000 sits 22%
// below the former and 28% above the latter — ordinary deflate drift between
// Go releases cannot flip either comparison unnoticed, and a change large
// enough to do so fails this test loudly and the constant gets remeasured.
func TestTranscodeWebPToPNGDownsamplesWhenTheFullSizeEncodeExceedsTheCeiling(t *testing.T) {
	const downsampleCeiling = 184_000 // straddles katharina's full-size (236,281) and 384-rung (144,264) attempts
	payload := readWebPSample(t, "katharina.webp")
	out, err := transcodeWebPToPNG(bytes.NewReader(payload), downsampleCeiling)
	if err != nil {
		t.Fatalf("transcodeWebPToPNG() error = %v", err)
	}
	if len(out) > downsampleCeiling {
		t.Errorf("encoded PNG is %d bytes, over the %d-byte test ceiling", len(out), downsampleCeiling)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decoding the downsampled result as PNG: %v", err)
	}
	if want := image.Rect(0, 0, AvatarSize*3/4, AvatarSize*3/4); img.Bounds() != want {
		t.Errorf("bounds = %v, want %v — the full-size rung should have been rejected and the 384 rung used", img.Bounds(), want)
	}
}
