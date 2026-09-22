package avatar

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

// AvatarSize is the edge length, in pixels, of the square canvas every
// transcoded avatar PNG is scaled to.
const AvatarSize = 512

// MaxPNGBytes is the byte ceiling on one encoded avatar PNG: 1 MiB
// (1,048,576 bytes).
const MaxPNGBytes = 1 << 20

// ErrAvatarTooLarge reports that downsampling ran out of room: every edge
// length in the reduction ladder still encoded a PNG above the byte ceiling.
var ErrAvatarTooLarge = errors.New("avatar: transcoded PNG exceeds the byte ceiling")

// TranscodeWebPToPNG reads one WebP image from r (lossy VP8, lossless VP8L,
// or extended VP8X — whatever golang.org/x/image/webp decodes), scales the
// decoded frame to AvatarSize x AvatarSize, and encodes it as PNG. It
// returns the encoded bytes when the PNG fits under MaxPNGBytes, a wrapped
// decode error when r does not hold a decodable WebP image, and
// ErrAvatarTooLarge when every downsampling attempt still exceeds the
// ceiling. The 1 MiB bound is what lets an avatar be handed to
// AccountsService as one bounded write instead of a streamed unknown.
func TranscodeWebPToPNG(r io.Reader) ([]byte, error) {
	return transcodeWebPToPNG(r, MaxPNGBytes)
}

// transcodeWebPToPNG is the testable core of TranscodeWebPToPNG: ceiling is
// the byte budget every attempt must fit, the seam through which tests reach
// the ErrAvatarTooLarge path with a budget no PNG can meet.
//
// The reduction ladder is load-bearing, not defensive. A 512x512 RGBA scanline
// stream is 512*(2048+1) = 1,049,088 bytes of PNG-filtered data before deflate
// even runs — already above the 1,048,576-byte ceiling — so content that does
// not compress can never fit at full size. Each rung below the first scales
// the decoded image to the next edge length and re-encodes; at AvatarSize/2
// the raw stream is 256*(1024+1) = 262,400 bytes, a quarter of the ceiling, so
// reduction essentially always succeeds, and ErrAvatarTooLarge stays as the
// explicit contract for an injected ceiling or a future ladder change.
func transcodeWebPToPNG(r io.Reader, ceiling int) ([]byte, error) {
	src, err := webp.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("avatar: decoding WebP: %w", err)
	}
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	for _, edge := range [3]int{AvatarSize, AvatarSize * 3 / 4, AvatarSize / 2} {
		dst := image.NewNRGBA(image.Rect(0, 0, edge, edge))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
		var buf bytes.Buffer
		if err := enc.Encode(&buf, dst); err != nil {
			return nil, fmt.Errorf("avatar: encoding PNG: %w", err)
		}
		if buf.Len() <= ceiling {
			return buf.Bytes(), nil
		}
	}
	return nil, ErrAvatarTooLarge
}
