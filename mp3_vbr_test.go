package audioduration

import (
	"bytes"
	"fmt"
	"testing"
)

// makeVBRFrames builds n frames whose bitrate alternates between the given
// indices, with NO Xing/VBRI header. This is the shape issue #2 describes: a
// genuinely variable-bitrate stream the size-division fallback cannot measure,
// because that fallback divides total size by the length of the FIRST frame
// alone.
func makeVBRFrames(specs []struct {
	idx  uint8
	kbps int
}, n int) []byte {
	var out []byte
	for i := 0; i < n; i++ {
		s := specs[i%len(specs)]
		frameLen := frameLenFor(s.kbps)
		f := make([]byte, frameLen)
		f[0] = 0xFF
		f[1] = 0xFB
		f[2] = s.idx << 4
		f[3] = 0x00
		out = append(out, f...)
	}
	return out
}

var vbrSpecs = []struct {
	idx  uint8
	kbps int
}{
	{bitrateIdx128, 128}, // 417 bytes/frame
	{bitrateIdx320, 320}, // 1044 bytes/frame
}

// TestMp3VBRWithoutXingCountsFrames covers issue #2. The stream holds exactly
// 200 frames alternating 128/320 kbps and carries no Xing/VBRI header.
//
// The correct duration is 200 frames * 1152 / 44100 = 5.2245s.
//
// The size-division fallback computes totalSize/firstFrameLen. Here that is
// (100*417 + 100*1044) / 417 = 350 frames, reporting ~9.14s -- a 75% overshoot
// on a file whose real length it never measured. No error, no flag, just a
// plausible-looking number.
func TestMp3VBRWithoutXingCountsFrames(t *testing.T) {
	const frames = 200
	data := bytes.Join([][]byte{
		makeID3v2Tag(make([]byte, 512)),
		makeVBRFrames(vbrSpecs, frames),
	}, nil)

	got, err := Mp3(bytes.NewReader(data))
	assertDuration(t, got, err, secondsFor(frames))
}

// TestMp3VBRWithoutXingIgnoresTagBytes isolates the second half of issue #2:
// the size-division fallback counts the ID3v2 tag bytes as though they were
// audio. A frame-chain walk starts at the first frame, so growing the tag must
// not change the reported duration by even a millisecond.
func TestMp3VBRWithoutXingIgnoresTagBytes(t *testing.T) {
	const frames = 120
	audio := makeVBRFrames(vbrSpecs, frames)

	var durations []float64
	for _, tagSize := range []int{64, 4096, 65536} {
		data := bytes.Join([][]byte{makeID3v2Tag(make([]byte, tagSize)), audio}, nil)
		got, err := Mp3(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("tag size %d: %v", tagSize, err)
		}
		durations = append(durations, got)
		assertDuration(t, got, err, secondsFor(frames))
	}
	for i := 1; i < len(durations); i++ {
		if durations[i] != durations[0] {
			t.Fatalf("duration varies with ID3v2 tag size: %v", durations)
		}
	}
}

// TestMp3CBRWithoutXingUnchanged is the regression control the issue's
// acceptance criteria require: for a constant-bitrate stream the frame walk and
// the size division agree, so CBR files must be unaffected.
func TestMp3CBRWithoutXingUnchanged(t *testing.T) {
	const frames = 150
	data := bytes.Join([][]byte{
		makeID3v2Tag(make([]byte, 256)),
		makeFrames(bitrateIdx128, 128, frames, false, 0),
	}, nil)

	got, err := Mp3(bytes.NewReader(data))
	assertDuration(t, got, err, secondsFor(frames))
}

// TestMp3XingStillWins confirms the walk is only a FALLBACK. When a Xing header
// declares a frame count, that count is authoritative and no walk happens --
// the declared count here deliberately disagrees with the frames present.
func TestMp3XingStillWins(t *testing.T) {
	const present = 100
	const declared = 777
	data := bytes.Join([][]byte{
		makeID3v2Tag(make([]byte, 128)),
		makeFrames(bitrateIdx128, 128, present, true, declared),
	}, nil)

	got, err := Mp3(bytes.NewReader(data))
	assertDuration(t, got, err, secondsFor(declared))
}

// BenchmarkMp3VBRWalk measures the cost issue #2 asks to be measured before
// committing to the walk: roughly 10,800 header reads for an ~11MB file.
func BenchmarkMp3VBRWalk(b *testing.B) {
	// ~10,800 frames alternating bitrate, which is the scale the issue cites.
	data := bytes.Join([][]byte{
		makeID3v2Tag(make([]byte, 20480)),
		makeVBRFrames(vbrSpecs, 10800),
	}, nil)
	b.SetBytes(int64(len(data)))
	r := bytes.NewReader(data)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Mp3(r); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	fmt.Printf("fixture size: %d bytes\n", len(data))
}
