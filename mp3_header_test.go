package audioduration

import (
	"bytes"
	"math"
	"testing"
)

// Direct tests for the two helpers extracted by the false-sync fix. Coverage
// elsewhere reaches them only through Mp3, which exercises the ACCEPT path and
// leaves each individual rejection rule asserted only incidentally. These pin
// the rules themselves, so a future change that widens one fails here rather
// than silently re-admitting the false syncs the fix exists to reject.

// hdr assembles the 4 header bytes from the field values, mirroring the layout
// documented in mp3_test.go:
//
//	byte 0: 1111 1111
//	byte 1: 111 BB CC D    BB=version, CC=layer, D=protection
//	byte 2: EEEE FF G H    EEEE=bitrate index, FF=sample-rate index, G=padding
//	byte 3: II JJ K L MM   mode in the top 2 bits
//
// Padding is always 0 here: none of these cases turn on it, and the frame
// lengths asserted below are the unpadded ones.
func hdr(version, layer, bitrateIdx, sampleIdx uint8) [4]byte {
	return [4]byte{
		0xFF,
		0b11100000 | (version << 3) | (layer << 1) | 1,
		(bitrateIdx << 4) | (sampleIdx << 2),
		0x00,
	}
}

func TestParseFrameHeaderRejects(t *testing.T) {
	// Each case is a byte run that LOOKS like a frame header -- the sync word is
	// present in all but the first two -- but must be rejected. These are
	// exactly the shapes a false sync produces inside tag payloads and filler.
	cases := []struct {
		name string
		b    [4]byte
	}{
		{
			// The 0xFF filler case from issue #1: FF FF matches the 11-bit sync
			// pattern and carries reserved bitrate index 15.
			name: "all 0xFF filler",
			b:    [4]byte{0xFF, 0xFF, 0xFF, 0xFF},
		},
		{
			name: "no sync word",
			b:    [4]byte{0x00, 0x00, 0x00, 0x00},
		},
		{
			// The JPEG APP0 marker from issue #1's stacked-tag case.
			name: "jpeg APP0 marker",
			b:    [4]byte{0xFF, 0xE0, 0x00, 0x10},
		},
		{
			name: "second sync byte only 10 bits set",
			b:    [4]byte{0xFF, 0b11011111, 0x90, 0x00},
		},
		{
			// Version index 0b01 is reserved in the MPEG spec.
			name: "reserved mpeg version",
			b:    hdr(0b01, layerIII, bitrateIdx128, 0),
		},
		{
			// Layer index 0b00 is reserved.
			name: "reserved layer",
			b:    hdr(mpeg1, 0b00, bitrateIdx128, 0),
		},
		{
			// Index 0 is "free format": the frame length is not derivable from
			// the header, so it cannot anchor a scan.
			name: "free-format bitrate index 0",
			b:    hdr(mpeg1, layerIII, 0, 0),
		},
		{
			name: "reserved bitrate index 15",
			b:    hdr(mpeg1, layerIII, 15, 0),
		},
		{
			// Sample-rate index 3 is reserved for every MPEG version.
			name: "reserved sample rate index 3",
			b:    hdr(mpeg1, layerIII, bitrateIdx128, 3),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := parseFrameHeader(tc.b); ok {
				t.Errorf("parseFrameHeader(% x) accepted a header it must reject", tc.b)
			}
		})
	}
}

func TestParseFrameHeaderAccepts(t *testing.T) {
	// The control: a well-formed MPEG-1 Layer III 128 kbps 44100 Hz frame must
	// be accepted, with its fields decoded. Without this the rejection table
	// above would pass just as well against a parser that rejects everything.
	h, ok := parseFrameHeader(hdr(mpeg1, layerIII, bitrateIdx128, 0))
	if !ok {
		t.Fatal("parseFrameHeader rejected a valid MPEG-1 Layer III frame")
	}
	if h.bitRate != 128 {
		t.Errorf("bitRate = %d, want 128", h.bitRate)
	}
	if h.sampleRate != testSampleRate {
		t.Errorf("sampleRate = %d, want %d", h.sampleRate, testSampleRate)
	}
	if h.samplesPerFrame != testSamplesPerFrame {
		t.Errorf("samplesPerFrame = %d, want %d", h.samplesPerFrame, testSamplesPerFrame)
	}
	if want := frameLenFor(128); h.frameLen != want {
		t.Errorf("frameLen = %d, want %d", h.frameLen, want)
	}
}

// TestParseFrameHeaderMinFrameLen pins the minFrameLen floor to the EXACT legal
// minimum rather than a conservative guess. MPEG-2 Layer III at 8 kbps and
// 24000 Hz gives (576/8)*8000/24000 = 24 bytes, so the smallest legal frame in
// the format must be accepted. A floor raised above 24 would reject it.
func TestParseFrameHeaderMinFrameLen(t *testing.T) {
	// Bitrate index 1 in the MPEG-2 Layer III table is 8 kbps; sample-rate
	// index 1 for MPEG-2 is 24000 Hz.
	h, ok := parseFrameHeader(hdr(mpeg2, layerIII, 1, 1))
	if !ok {
		t.Fatal("parseFrameHeader rejected the smallest legal frame (8 kbps MPEG-2 Layer III at 24000 Hz)")
	}
	if h.frameLen != minFrameLen {
		t.Errorf("frameLen = %d, want exactly minFrameLen (%d)", h.frameLen, minFrameLen)
	}
}

// TestMp3ShortTailFallsBackToWalk covers the case where the stream ends before
// the Xing/VBRI marker slot. findFirstFrame has already validated a real frame,
// so the audio is countable even though the metadata slot is not there; Mp3 must
// count it rather than returning the read error.
//
// The fixture is the smallest stream that reaches this: ONE minimum-size
// MPEG-2 Layer III frame. At 8 kbps / 24000 Hz the frame is 24 bytes, and
// stereo MPEG-2 Layer III side info is 17 bytes, so after the 4-byte header
// and the side info only 3 bytes remain -- one short of the 4-byte marker read.
// The single frame is also the last, which findFirstFrame accepts as the final
// frame rather than requiring a chaining successor.
func TestMp3ShortTailFallsBackToWalk(t *testing.T) {
	const (
		mpeg2LIIISamples    = 576
		mpeg2LIIISampleRate = 24000
	)

	frame := make([]byte, minFrameLen)
	// bitrate index 1 = 8 kbps, sample-rate index 1 = 24000 Hz for MPEG-2.
	h := hdr(mpeg2, layerIII, 1, 1)
	copy(frame, h[:])

	got, err := Mp3(bytes.NewReader(frame))
	if err != nil {
		t.Fatalf("Mp3 returned an error on a stream whose frame is valid but whose tail is too short for a Xing/VBRI marker: %v", err)
	}

	want := float64(mpeg2LIIISamples) / float64(mpeg2LIIISampleRate)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("duration = %v, want %v (one %d-sample frame at %d Hz)",
			got, want, mpeg2LIIISamples, mpeg2LIIISampleRate)
	}
}

// TestFindFirstFrameNoFrame asserts the error path, which no other test reaches:
// a stream containing no decodable frame must report that rather than returning
// a bogus offset.
func TestFindFirstFrameNoFrame(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		// 0xFF everywhere means a sync candidate at every offset, all of which
		// must be rejected -- the scan must exhaust rather than accept one.
		{name: "all 0xFF", data: bytes.Repeat([]byte{0xFF}, 4096)},
		{name: "all zero", data: make([]byte, 4096)},
		// Shorter than a single header.
		{name: "too short", data: []byte{0xFF, 0xFB}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := findFirstFrame(bytes.NewReader(tc.data), 0)
			if err == nil {
				t.Fatal("findFirstFrame accepted a stream with no valid frame")
			}
		})
	}
}
