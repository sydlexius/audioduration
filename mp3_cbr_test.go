package audioduration

import (
	"bytes"
	"io"
	"testing"
)

// These cover the constant-bitrate fast path: a stream with no Xing/VBRI header
// whose frames all report one bitrate is measured by dividing its audio byte
// range by that bitrate, which is exact, instead of walking every frame header
// to end of file.
//
// The fixtures follow mp3_test.go's conventions -- MPEG-1 Layer III, 44100 Hz,
// stereo, built in memory from synthetic headers over a zeroed payload.

// countingReader wraps an io.ReadSeeker and records how many bytes were
// actually read through it. Seeking is free, so this measures exactly the
// quantity that matters to a caller whose reader is backed by slow storage: how
// much of the file had to be pulled off disk.
type countingReader struct {
	r     io.ReadSeeker
	bytes int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.bytes += int64(n)
	return n, err
}

func (c *countingReader) Seek(offset int64, whence int) (int64, error) {
	return c.r.Seek(offset, whence)
}

// measure runs Mp3 over data through a counting reader and returns the duration
// and the bytes read.
func measure(t *testing.T, data []byte) (float64, int64) {
	t.Helper()
	c := &countingReader{r: bytes.NewReader(data)}
	d, err := Mp3(c)
	if err != nil {
		t.Fatalf("Mp3: %v", err)
	}
	return d, c.bytes
}

// makePaddedCBRFrames builds n constant-bitrate frames that use the padding bit
// the way a real encoder does.
//
// At 128 kbps / 44100 Hz a frame is 417.959 bytes, which is not an integer, so
// the encoder emits 417-byte frames and pads to 418 often enough for the average
// to land on the nominal rate. That makes frame LENGTH vary within a stream that
// is unambiguously constant BITRATE -- the exact trap a CBR check must not fall
// into.
func makePaddedCBRFrames(bitrateIdx uint8, kbps, n int) []byte {
	const base = float64(testSamplesPerFrame) / 8
	exact := base * float64(kbps*1000) / float64(testSampleRate)
	whole := int(exact)
	frac := exact - float64(whole)

	var out []byte
	acc := 0.0
	for i := 0; i < n; i++ {
		var padding uint8
		length := whole
		acc += frac
		if acc >= 1 {
			acc--
			padding = 1
			length++
		}
		f := make([]byte, length)
		f[0] = 0xFF
		f[1] = 0xFB
		f[2] = bitrateIdx<<4 | padding<<1
		f[3] = 0x00
		out = append(out, f...)
	}
	return out
}

// makeID3v1Tag returns a 128-byte ID3v1 trailer.
func makeID3v1Tag() []byte {
	tag := make([]byte, id3v1Len)
	copy(tag, "TAG")
	copy(tag[3:], "a title")
	return tag
}

// TestMp3CBRWithoutXingIsCheap is the regression test for the whole fix. A
// constant-bitrate stream with no Xing header must be measured WITHOUT reading
// the file, because a consumer caching durations across a large library relies
// on a duration probe touching only a file's header region.
//
// The bound is an upper limit on bytes read, not an exact count, so an
// implementation change that stays cheap does not have to update it -- but a
// change that reverts to reading the whole stream fails loudly.
func TestMp3CBRWithoutXingIsCheap(t *testing.T) {
	const frames = 12000 // ~5 MB at 128 kbps, 313s of audio
	data := bytes.Join([][]byte{
		makeID3v2Tag(make([]byte, 4096)),
		makePaddedCBRFrames(bitrateIdx128, 128, frames),
	}, nil)

	got, read := measure(t, data)
	assertDuration(t, got, nil, secondsFor(frames))

	// 256 KiB leaves room for the 64 KiB first-frame scan block plus every
	// probe window, while being a small fraction of the ~5 MB fixture. Reading
	// the whole stream, as the frame walk does, is ~20x this.
	const maxRead = 256 * 1024
	if read > maxRead {
		t.Fatalf("read %d bytes of a %d-byte CBR stream, want <= %d: the O(1) path was not taken",
			read, len(data), maxRead)
	}
	// Guard the bound from the other side: if the fixture ever shrinks below
	// the bound the assertion above proves nothing.
	if int64(len(data)) < 4*maxRead {
		t.Fatalf("fixture is %d bytes, too small for a %d-byte bound to be meaningful", len(data), maxRead)
	}
}

// TestMp3CBRPaddedFramesStayCheap is the padding trap, checked at two bitrates
// whose exact frame length is fractional: 417.959 bytes at 128 kbps and 1044.9
// at 320. Frame LENGTH therefore alternates within each stream while the bitrate
// never changes, so a check comparing length instead of bitrate would classify
// every real CBR file as variable and read all of it.
func TestMp3CBRPaddedFramesStayCheap(t *testing.T) {
	const frames = 12000
	cases := []struct {
		name string
		idx  uint8
		kbps int
	}{
		{"128kbps", bitrateIdx128, 128},
		{"320kbps", bitrateIdx320, 320},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := bytes.Join([][]byte{
				makeID3v2Tag(make([]byte, 1024)),
				makePaddedCBRFrames(tc.idx, tc.kbps, frames),
			}, nil)

			got, read := measure(t, data)
			assertDuration(t, got, nil, secondsFor(frames))

			const maxRead = 256 * 1024
			if read > maxRead {
				t.Fatalf("padded CBR stream read %d bytes, want <= %d: padding was misread as bitrate variation",
					read, maxRead)
			}
		})
	}
}

// TestMp3VBRWithoutXingStillWalks is the control that pins the v0.9.0 fix in
// place. A genuinely variable stream must still be frame-counted, which means it
// must still be read -- the cheap path must NOT claim it.
func TestMp3VBRWithoutXingStillWalks(t *testing.T) {
	const frames = 12000
	data := bytes.Join([][]byte{
		makeID3v2Tag(make([]byte, 1024)),
		makeVBRFrames(vbrSpecs, frames),
	}, nil)

	got, read := measure(t, data)
	assertDuration(t, got, nil, secondsFor(frames))

	// Size division here would report roughly 75% too long. Assert the walk
	// actually happened, so a future change cannot pass the duration check by
	// coincidence while skipping the walk.
	if read < int64(len(data))/2 {
		t.Fatalf("VBR stream read only %d of %d bytes: it was not frame-counted", read, len(data))
	}
}

// TestMp3ConstantHeadVariableTailIsWalked covers the case the spread probes
// exist for: a stream that is constant for its first thousands of frames and
// changes bitrate only later, which a head-only check would misclassify as CBR
// and then measure with the wrong divisor.
func TestMp3ConstantHeadVariableTailIsWalked(t *testing.T) {
	const head = 6000
	const tail = 6000
	data := bytes.Join([][]byte{
		makeID3v2Tag(make([]byte, 512)),
		makeFrames(bitrateIdx128, 128, head, false, 0),
		makeFrames(bitrateIdx320, 320, tail, false, 0),
	}, nil)

	got, _ := measure(t, data)
	// Every frame carries 1152 samples regardless of bitrate, so the true
	// duration is simply the total frame count.
	assertDuration(t, got, nil, secondsFor(head+tail))
}

// TestMp3CBRIgnoresID3v1Trailer covers the trailer trap. Size division over the
// whole file would count an ID3v1 tag's 128 bytes as audio; over a low-bitrate
// stream a stacked APE + Lyrics3 + ID3v1 tail is seconds of phantom duration.
// Growing the trailer must not move the duration at all.
func TestMp3CBRIgnoresID3v1Trailer(t *testing.T) {
	const frames = 12000
	audio := bytes.Join([][]byte{
		makeID3v2Tag(make([]byte, 512)),
		makePaddedCBRFrames(bitrateIdx128, 128, frames),
	}, nil)

	bare, _ := measure(t, audio)
	tagged, read := measure(t, append(append([]byte{}, audio...), makeID3v1Tag()...))

	assertDuration(t, tagged, nil, secondsFor(frames))
	if bare != tagged {
		t.Fatalf("ID3v1 trailer changed the duration: %.6f without, %.6f with", bare, tagged)
	}
	const maxRead = 256 * 1024
	if read > maxRead {
		t.Fatalf("trailer handling cost the fast path: read %d bytes, want <= %d", read, maxRead)
	}
}

// TestMp3CBRIgnoresAPEAndLyrics3Trailers checks that stacked trailers peel. A
// file commonly ends Lyrics3v2, then APE, then ID3v1, and only peeling all
// three leaves the true audio range.
func TestMp3CBRIgnoresAPEAndLyrics3Trailers(t *testing.T) {
	const frames = 12000
	audio := bytes.Join([][]byte{
		makeID3v2Tag(make([]byte, 512)),
		makePaddedCBRFrames(bitrateIdx128, 128, frames),
	}, nil)
	bare, _ := measure(t, audio)

	// Lyrics3v2: body, then a 6-digit ASCII size covering the body, then the
	// terminator.
	lyricsBody := append([]byte("LYRICSBEGIN"), make([]byte, 500)...)
	lyrics := append(lyricsBody, []byte("000511LYRICS200")...)

	// APE tag: 32-byte footer whose size field covers body + footer, no header
	// flag set.
	apeBody := make([]byte, 200)
	ape := append(apeBody, makeAPEFooter(len(apeBody)+apeFooterLen)...)

	data := bytes.Join([][]byte{audio, lyrics, ape, makeID3v1Tag()}, nil)

	got, _ := measure(t, data)
	assertDuration(t, got, nil, secondsFor(frames))
	if got != bare {
		t.Fatalf("stacked trailers changed the duration: %.6f bare, %.6f with trailers", bare, got)
	}
}

// makeAPEFooter builds a 32-byte APE tag footer declaring the given total size
// (tag body plus this footer), with no header present.
func makeAPEFooter(total int) []byte {
	f := make([]byte, apeFooterLen)
	copy(f, "APETAGEX")
	putUint32LE(f[8:], 2000) // version
	putUint32LE(f[12:], uint32(total))
	putUint32LE(f[16:], 1) // item count
	putUint32LE(f[20:], 0) // flags: no header
	return f
}

func putUint32LE(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

// TestMp3XingUnchangedByCBRPath confirms the Xing fast path is untouched: a
// declared frame count still wins outright, and still costs only the head of the
// file. The declared count deliberately disagrees with the frames present.
func TestMp3XingUnchangedByCBRPath(t *testing.T) {
	const present = 12000
	const declared = 777
	data := bytes.Join([][]byte{
		makeID3v2Tag(make([]byte, 512)),
		makeFrames(bitrateIdx128, 128, present, true, declared),
	}, nil)

	got, read := measure(t, data)
	assertDuration(t, got, nil, secondsFor(declared))

	const maxRead = 128 * 1024
	if read > maxRead {
		t.Fatalf("Xing path read %d bytes, want <= %d", read, maxRead)
	}
}

// TestMp3ShortCBRStreamWalks pins the small-file rule: below the probe's own
// I/O footprint there is nothing to save, so the exact walk is used. It must
// still return the right answer.
func TestMp3ShortCBRStreamWalks(t *testing.T) {
	const frames = 8
	data := bytes.Join([][]byte{
		makeID3v2Tag(make([]byte, 64)),
		makePaddedCBRFrames(bitrateIdx128, 128, frames),
	}, nil)

	got, read := measure(t, data)
	assertDuration(t, got, nil, secondsFor(frames))
	if read == 0 {
		t.Fatal("short stream read nothing")
	}
}

// TestMp3GarbageMidStreamIsNotCBR covers a resync region. A run of non-frame
// bytes spliced into an otherwise constant stream must not be measured by size
// division, which would count the garbage as audio. The probes fail to chain
// there, so the walk takes over and reports only the audio that is really
// present.
func TestMp3GarbageMidStreamIsNotCBR(t *testing.T) {
	const head = 6000
	const tail = 6000
	garbage := bytes.Repeat([]byte{0x00}, 200*1024)
	data := bytes.Join([][]byte{
		makeID3v2Tag(make([]byte, 512)),
		makeFrames(bitrateIdx128, 128, head, false, 0),
		garbage,
		makeFrames(bitrateIdx128, 128, tail, false, 0),
	}, nil)

	got, _ := measure(t, data)
	// Only the real frames count; the garbage contributes nothing. Size
	// division over the full range would add the garbage's 200 KiB as ~12.8s.
	assertDuration(t, got, nil, secondsFor(head+tail))
}

// TestAudioEndOffsetRejectsCorruptTrailerSizes pins the guard on the trailer
// peeler's arithmetic: a size field larger than the stream must be ignored
// rather than driving the audio end negative.
func TestAudioEndOffsetRejectsCorruptTrailerSizes(t *testing.T) {
	audio := makeFrames(bitrateIdx128, 128, 20, false, 0)

	// APE footer claiming a tag far larger than the file.
	corrupt := append(append([]byte{}, audio...), makeAPEFooter(1<<30)...)
	end, err := audioEndOffset(bytes.NewReader(corrupt))
	if err != nil {
		t.Fatalf("audioEndOffset: %v", err)
	}
	if end != int64(len(corrupt)) {
		t.Fatalf("corrupt APE size moved the audio end to %d, want %d (unchanged)", end, len(corrupt))
	}

	// Lyrics3 terminator whose size digits are not digits at all.
	bogus := append(append([]byte{}, audio...), []byte("ab!defLYRICS200")...)
	end, err = audioEndOffset(bytes.NewReader(bogus))
	if err != nil {
		t.Fatalf("audioEndOffset: %v", err)
	}
	if end != int64(len(bogus)) {
		t.Fatalf("non-numeric Lyrics3 size moved the audio end to %d, want %d (unchanged)", end, len(bogus))
	}
}
