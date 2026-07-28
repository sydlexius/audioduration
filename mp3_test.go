package audioduration

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// These fixtures are built in memory, byte by byte, with zero committed assets.
// That is possible because Mp3 takes an io.ReadSeeker and reads only frame
// headers -- it never decodes audio -- so synthetic headers over a zeroed
// payload are indistinguishable from real audio for duration purposes.
//
// Every frame below is MPEG-1 Layer III, 44100 Hz, stereo, no padding, no CRC.
//
//	byte 0: 1111 1111                                    = 0xFF
//	byte 1: 111 BB CC D   BB=11 (MPEG-1) CC=01 (LIII) D=1 (no CRC) = 0xFB
//	byte 2: EEEE FF G H   EEEE=bitrate index, FF=00 (44100), G=0, H=0
//	byte 3: II JJ K L MM  all zero (stereo, no emphasis)
const (
	testSampleRate      = 44100
	testSamplesPerFrame = 1152
	// bitrate index 9 in the MPEG-1 Layer III table is 128 kbps.
	bitrateIdx128 = 9
	// bitrate index 14 in the same table is 320 kbps.
	bitrateIdx320 = 14
	// bitrate index 1 in the same table is 32 kbps, the lowest MPEG-1 Layer III
	// rate. Paired with 320 it gives the widest legal ratio between two frames.
	bitrateIdx32 = 1
)

// frameLenFor returns the byte length of one frame at the given kbps, matching
// the frameLength formula for MPEG-1 Layer III with no padding.
func frameLenFor(kbps int) int {
	return (testSamplesPerFrame / 8) * kbps * 1000 / testSampleRate
}

// secondsFor returns the exact duration of n frames at 44100 Hz / 1152 samples.
func secondsFor(n int) float64 {
	return float64(testSamplesPerFrame) / float64(testSampleRate) * float64(n)
}

// makeFrames builds n consecutive frames at the given bitrate index. When
// withXing is true the first frame carries a Xing header declaring xingFrames
// total frames, which is what a real encoder writes and what lets a test assert
// an exact duration without depending on the no-header fallback path.
func makeFrames(bitrateIdx uint8, kbps, n int, withXing bool, xingFrames uint32) []byte {
	frameLen := frameLenFor(kbps)
	out := make([]byte, 0, frameLen*n)
	for i := 0; i < n; i++ {
		f := make([]byte, frameLen)
		f[0] = 0xFF
		f[1] = 0xFB
		f[2] = bitrateIdx << 4 // sample rate index 00 = 44100, no padding
		f[3] = 0x00
		if i == 0 && withXing {
			// 4-byte header + 32 bytes of MPEG-1 stereo side info, then "Xing".
			copy(f[36:], "Xing")
			binary.BigEndian.PutUint32(f[40:], 0x00000001) // flags: frames field present
			binary.BigEndian.PutUint32(f[44:], xingFrames)
		}
		out = append(out, f...)
	}
	return out
}

// makeID3v2Tag wraps payload in a minimal ID3v2.4 tag header with a synchsafe
// size field.
func makeID3v2Tag(payload []byte) []byte {
	tag := make([]byte, 10, 10+len(payload))
	copy(tag, "ID3")
	tag[3], tag[4] = 0x04, 0x00 // version 2.4.0
	tag[5] = 0x00               // no flags, so no extended header
	size := len(payload)
	for i := 0; i < 4; i++ {
		tag[9-i] = byte((size >> (7 * i)) & 0x7F)
	}
	return append(tag, payload...)
}

// jpegArtPayload returns bytes that open like an embedded JPEG. The APP0 marker
// FF E0 is a false MPEG sync: 0xE0>>5 is 0b111, so the 11-bit sync pattern
// matches, but the decoded header is illegal (reserved layer).
func jpegArtPayload(size int) []byte {
	p := make([]byte, size)
	copy(p, []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'})
	return p
}

func assertDuration(t *testing.T, got float64, err error, want float64) {
	t.Helper()
	if err != nil {
		t.Fatalf("Mp3 returned error %v, want duration %.4fs", err, want)
	}
	if math.Abs(got-want) > 0.05 {
		t.Fatalf("Mp3 = %.4fs, want %.4fs (delta %.4fs)", got, want, math.Abs(got-want))
	}
}

// TestMp3StackedTagsWithFalseSyncInArt covers issue #1 trigger (a): only the
// first ID3v2 tag is skipped, so the scan walks into the second tag's payload
// and misreads the JPEG APP0 marker as a frame header.
func TestMp3StackedTagsWithFalseSyncInArt(t *testing.T) {
	const frames = 100
	data := bytes.Join([][]byte{
		makeID3v2Tag(make([]byte, 512)),
		makeID3v2Tag(jpegArtPayload(2048)),
		makeFrames(bitrateIdx128, 128, frames, true, frames),
	}, nil)

	got, err := Mp3(bytes.NewReader(data))
	assertDuration(t, got, err, secondsFor(frames))
}

// TestMp3StackedTagsWithValidLookingFrameInArt is the discriminator for the
// tag-loop half of the fix. The second tag's payload holds a run of frames that
// are individually legal AND chain correctly, so candidate validation alone
// accepts them and reports a wrong duration. Only skipping every consecutive
// ID3v2 tag keeps the scan out of the payload.
func TestMp3StackedTagsWithValidLookingFrameInArt(t *testing.T) {
	const frames = 100
	// Three legal, self-chaining 320 kbps frames buried in the second tag.
	decoy := makeFrames(bitrateIdx320, 320, 3, false, 0)
	payload := make([]byte, 64+len(decoy))
	copy(payload[64:], decoy)

	data := bytes.Join([][]byte{
		makeID3v2Tag(make([]byte, 128)),
		makeID3v2Tag(payload),
		makeFrames(bitrateIdx128, 128, frames, true, frames),
	}, nil)

	got, err := Mp3(bytes.NewReader(data))
	assertDuration(t, got, err, secondsFor(frames))
}

// TestMp3FillerBeforeFirstFrame covers issue #1 trigger (b): 0xFF filler between
// the tag end and the first real frame. FF FF matches the sync pattern and
// carries reserved bitrate index 15. No tag loop can fix this one -- the filler
// sits outside every tag -- so it isolates the candidate-validation half.
func TestMp3FillerBeforeFirstFrame(t *testing.T) {
	const frames = 100
	filler := bytes.Repeat([]byte{0xFF}, 1024)
	data := bytes.Join([][]byte{
		makeID3v2Tag(make([]byte, 256)),
		filler,
		makeFrames(bitrateIdx128, 128, frames, true, frames),
	}, nil)

	got, err := Mp3(bytes.NewReader(data))
	assertDuration(t, got, err, secondsFor(frames))
}

// TestMp3StackedTagsNoFalseSync is the control the issue calls out: stacked tags
// WITHOUT a false sync pass even unpatched. It isolates the real trigger from
// the plausible-but-wrong one ("multiple ID3v2 tags").
func TestMp3StackedTagsNoFalseSync(t *testing.T) {
	const frames = 100
	data := bytes.Join([][]byte{
		makeID3v2Tag(make([]byte, 512)),
		makeID3v2Tag(make([]byte, 2048)), // all zero, no 0xFF anywhere
		makeFrames(bitrateIdx128, 128, frames, true, frames),
	}, nil)

	got, err := Mp3(bytes.NewReader(data))
	assertDuration(t, got, err, secondsFor(frames))
}

// TestMp3SingleTagPlainFrames is the baseline control: one tag, clean frames.
func TestMp3SingleTagPlainFrames(t *testing.T) {
	const frames = 100
	data := bytes.Join([][]byte{
		makeID3v2Tag(make([]byte, 512)),
		makeFrames(bitrateIdx128, 128, frames, true, frames),
	}, nil)

	got, err := Mp3(bytes.NewReader(data))
	assertDuration(t, got, err, secondsFor(frames))
}

// TestMp3NoID3Tag confirms the no-tag path still works.
func TestMp3NoID3Tag(t *testing.T) {
	const frames = 50
	data := makeFrames(bitrateIdx128, 128, frames, true, frames)

	got, err := Mp3(bytes.NewReader(data))
	assertDuration(t, got, err, secondsFor(frames))
}
