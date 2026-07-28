package audioduration

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
)

const (
	mpeg1  = 0b11
	mpeg2  = 0b10
	mpeg25 = 0b00
)

const (
	layerI   = 0b11
	layerII  = 0b10
	layerIII = 0b01
)

const (
	stereo        = 0b00
	jointStereo   = 0b01
	dualChannel   = 0b10
	singleChannel = 0b11
)

// getSampleRate Lookup sample rate.
// https://www.codeproject.com/Articles/8295/MPEG-Audio-Frame-Header#SamplingRate
func getSampleRate(mpegVer, sampleRateIndex uint8) int {
	if sampleRateIndex > 2 {
		return 0
	}

	var sampleRate int
	switch mpegVer {
	case mpeg2:
		sampleRate = []int{22050, 24000, 16000}[sampleRateIndex]
	case mpeg25:
		sampleRate = []int{11025, 12000, 8000}[sampleRateIndex]
	default:
		sampleRate = []int{44100, 48000, 32000}[sampleRateIndex]
	}
	return sampleRate
}

// getBitRate Lookup bit rate.
// https://www.codeproject.com/Articles/8295/MPEG-Audio-Frame-Header#Bitrate
func getBitRate(mpegVer, layer, bitRateIndex uint8) int {
	// Validate bitRateIndex to prevent array out of bounds
	if bitRateIndex > 15 {
		return 0
	}

	var layerIdx int
	switch layer {
	case layerI:
		layerIdx = 0
	case layerII:
		layerIdx = 1
	case layerIII:
		layerIdx = 2
	default:
		return 0
	}

	mpeg1BitRateTable := [][]int{
		{0, 32, 64, 96, 128, 160, 192,
			224, 256, 288, 320, 352, 384, 416, 448, 0}, // Layer I
		{0, 32, 48, 56, 64, 80, 96, 112,
			128, 160, 192, 224, 256, 320, 384, 0}, // Layer II
		{0, 32, 40, 48, 56, 64, 80, 96,
			112, 128, 160, 192, 224, 256, 320, 0}, // Layer III
	}
	mpeg2BitRateTable := [][]int{
		{0, 32, 48, 56, 64, 80, 96, 112,
			128, 144, 160, 176, 192, 224, 256, 0}, // Layer I
		{0, 8, 16, 24, 32, 40, 48, 56,
			64, 80, 96, 112, 128, 144, 160, 0}, // Layer II
		{0, 8, 16, 24, 32, 40, 48, 56,
			64, 80, 96, 112, 128, 144, 160, 0}, // Layer III
	}
	switch mpegVer {
	case mpeg1:
		return mpeg1BitRateTable[layerIdx][bitRateIndex]
	case mpeg2, mpeg25:
		return mpeg2BitRateTable[layerIdx][bitRateIndex]
	default:
		return 0
	}
}

// getSamples Lookup samples per frame.
// https://www.codeproject.com/Articles/8295/MPEG-Audio-Frame-Header#SamplesPerFrame
func getSamplesPerFrame(mpegVer, layer uint8) int {
	var samples int
	switch layer {
	case layerI:
		samples = 384
	case layerII:
		samples = 1152
	case layerIII:
		switch mpegVer {
		case mpeg1:
			samples = 1152
		case mpeg2, mpeg25:
			samples = 576
		}
	}
	return samples
}

// frameLength Calculate how many bytes in a frame. Notice the unit of bitRateK
// is Kbps(= 1000bps).
func frameLength(layer, padding uint8, samples, bitRateK, sampleRate int) int {
	var frameLen float64
	switch layer {
	case layerI:
		frameLen = (12*float64(bitRateK*1000)/float64(sampleRate) + float64(padding)) * 4
	case layerII, layerIII:
		frameLen = float64(samples/8)*float64(bitRateK*1000)/float64(sampleRate) + float64(padding)
	}
	return int(frameLen)
}

// getSideInfoLen Lookup side info length
// https://www.codeproject.com/Articles/8295/MPEG-Audio-Frame-Header#SideInfo
func getSideInfoLen(mpegVer, mode uint8) int64 {
	var sideInfoLen int64
	switch mode {
	case stereo, jointStereo, dualChannel:
		switch mpegVer {
		case mpeg1:
			sideInfoLen = 32
		case mpeg2, mpeg25:
			sideInfoLen = 17
		}
	case singleChannel:
		switch mpegVer {
		case mpeg1:
			sideInfoLen = 17
		case mpeg2, mpeg25:
			sideInfoLen = 9
		}
	}
	return sideInfoLen
}

// VBRI VBRI Header
type VBRI struct {
	totalSize  uint32
	totalFrame uint32
}

// Xing Xing Header
type Xing struct {
	flags      uint32
	totalFrame uint32
}

// parseVBRI Extract total frames in VBRI header.
// https://www.codeproject.com/Articles/8295/MPEG-Audio-Frame-Header#VBRIHeader
func parseVBRI(r io.ReadSeeker) (VBRI, error) {
	var vbri VBRI
	if _, err := r.Seek(10, io.SeekCurrent); err != nil {
		return vbri, err
	}
	buf4 := make([]byte, 4)
	if _, err := io.ReadFull(r, buf4); err != nil {
		return vbri, err
	}
	vbri.totalSize = binary.BigEndian.Uint32(buf4)
	if _, err := io.ReadFull(r, buf4); err != nil {
		return vbri, err
	}
	vbri.totalFrame = binary.BigEndian.Uint32(buf4)
	return vbri, nil
}

// parseXing Extract total frames in Xing header.
// https://www.codeproject.com/Articles/8295/MPEG-Audio-Frame-Header#XINGHeader
func parseXing(r io.ReadSeeker) (Xing, error) {
	var xing Xing
	buf4 := make([]byte, 4)
	if _, err := io.ReadFull(r, buf4); err != nil {
		return xing, err
	}
	xing.flags = binary.BigEndian.Uint32(buf4)
	if (xing.flags & 0x1) == 0 {
		return xing, errors.New("no frame info in Xing header")
	}
	if _, err := io.ReadFull(r, buf4); err != nil {
		return xing, err
	}
	xing.totalFrame = binary.BigEndian.Uint32(buf4)
	return xing, nil
}

// parseID3v2Length Parse ID3v2 tag length in ID3v2 tag header.
// https://id3.org/id3v2.4.0-structure
// http://fileformats.archiveteam.org/wiki/ID3#How_to_skip_past_an_ID3v2_segment
func parseID3v2Length(headbuf []byte) (offset int64) {
	// ID3v2 header must be at least 10 bytes
	if len(headbuf) < 10 {
		return 0
	}
	offset = 0
	for i := 6; i < 10; i++ {
		offset <<= 7
		// synchsafe: only low 7 bits are used per byte
		offset |= int64(headbuf[i] & 0x7F)
	}
	if (headbuf[5]>>4)&0b0001 == 1 {
		offset += 10
	}
	return
}

// minFrameLen is a sanity floor for a decoded frame length. MPEG-2/2.5 Layer II
// and III bitrates start at 8 kbps, so the smallest legal frame is 8 kbps at the
// highest MPEG-2 sample rate, 24000 Hz: (576/8)*8000/24000 = 24 bytes. The floor
// is therefore the EXACT legal minimum, not a conservative estimate -- it
// rejects the degenerate lengths a false sync produces while admitting every
// legal frame. Do not raise it: doing so would reject legal low-bitrate frames.
const minFrameLen = 24

// frameHeader is a decoded and validated 4-byte MPEG audio frame header.
type frameHeader struct {
	mpegVer         uint8
	layer           uint8
	protection      uint8
	mode            uint8
	bitRate         int
	sampleRate      int
	samplesPerFrame int
	frameLen        int
}

// parseFrameHeader decodes the 4 bytes of an MPEG audio frame header and
// reports whether they form a legal one. It is the single place that decides a
// sync candidate is real, so a caller scanning for the first frame and a caller
// walking the frame chain apply identical rules.
//
// ok is false when the sync word is absent or any decoded field is reserved or
// illegal. That is a REJECTION, not an error: the byte run merely looked like a
// header, which is expected inside tag payloads and filler.
func parseFrameHeader(b [4]byte) (frameHeader, bool) {
	var h frameHeader
	// 1111 1111, 111B BCCD, EEEE FFGH, IIJJ KLMM
	if b[0] != 0xFF || (b[1]>>5) != 0b111 {
		return h, false
	}
	h.mpegVer = (b[1] >> 3) & 0b00011
	h.layer = (b[1] & 0b00000110) >> 1
	h.protection = b[1] & 0x1
	h.mode = b[3] >> 6

	bitRateIndex := b[2] >> 4
	// Index 15 is reserved and index 0 means "free format", whose frame length
	// is not derivable from the header. getBitRate maps both to 0.
	h.bitRate = getBitRate(h.mpegVer, h.layer, bitRateIndex)
	if h.bitRate == 0 {
		return h, false
	}
	sampleFreqIndex := (b[2] >> 2) & 0b000011
	h.sampleRate = getSampleRate(h.mpegVer, sampleFreqIndex)
	if h.sampleRate == 0 {
		return h, false
	}
	h.samplesPerFrame = getSamplesPerFrame(h.mpegVer, h.layer)
	if h.samplesPerFrame == 0 {
		return h, false
	}
	padding := (b[2] >> 1) & 0b0000001
	h.frameLen = frameLength(h.layer, padding, h.samplesPerFrame, h.bitRate, h.sampleRate)
	if h.frameLen < minFrameLen {
		return h, false
	}
	return h, true
}

// readFrameHeaderAt reads 4 bytes at absolute offset pos and decodes them.
// The reader is left at an unspecified position; callers seek explicitly.
func readFrameHeaderAt(r io.ReadSeeker, pos int64) (frameHeader, bool) {
	if _, err := r.Seek(pos, io.SeekStart); err != nil {
		return frameHeader{}, false
	}
	var b [4]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return frameHeader{}, false
	}
	return parseFrameHeader(b)
}

// skipID3v2Tags advances past EVERY consecutive ID3v2 tag at the head of the
// stream and returns the offset of the first byte after them.
//
// Skipping only the first tag lets the frame scan walk into a later tag's
// payload, where embedded cover art supplies false syncs -- a JPEG APP0 marker
// (FF E0) matches the 11-bit sync pattern exactly.
func skipID3v2Tags(r io.ReadSeeker) (int64, error) {
	var pos int64
	head := make([]byte, 10)
	for {
		if _, err := r.Seek(pos, io.SeekStart); err != nil {
			return 0, err
		}
		if _, err := io.ReadFull(r, head); err != nil {
			// Fewer than 10 bytes remain, so no further tag can start here.
			// Not an error: the frame scan reports the real problem.
			return pos, nil
		}
		if string(head[0:3]) != "ID3" {
			return pos, nil
		}
		tagLen := parseID3v2Length(head)
		if tagLen <= 0 {
			// A zero-length tag would spin this loop forever.
			return pos + int64(len(head)), nil
		}
		pos += int64(len(head)) + tagLen
	}
}

// findFirstFrame scans forward from startPos for the first byte offset holding
// a real frame header, and returns that offset with the decoded header.
//
// A sync candidate is accepted only when it decodes legally AND the frame that
// should follow it also decodes legally (or the stream ends there, which is the
// last frame of a well-formed file). Requiring the chain is what rejects an
// isolated run of frame-shaped bytes; on rejection the scan resumes at the very
// next byte, never past it, so a real frame overlapping a false one is found.
func findFirstFrame(r io.ReadSeeker, startPos int64) (int64, frameHeader, error) {
	// Read in blocks rather than a byte at a time: a false sync can sit
	// thousands of bytes before the first real frame.
	const blockSize = 64 * 1024
	block := make([]byte, blockSize)

	pos := startPos
	for {
		if _, err := r.Seek(pos, io.SeekStart); err != nil {
			return 0, frameHeader{}, err
		}
		n, err := io.ReadFull(r, block)
		if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
			return 0, frameHeader{}, err
		}
		if n < 4 {
			return 0, frameHeader{}, errors.New("no valid mp3 frame found")
		}
		for i := 0; i+4 <= n; i++ {
			if block[i] != 0xFF {
				continue
			}
			h, ok := parseFrameHeader([4]byte{block[i], block[i+1], block[i+2], block[i+3]})
			if !ok {
				continue
			}
			candidate := pos + int64(i)
			// Require the successor frame to decode too. A stream that ends
			// exactly at the candidate's frame boundary is accepted as the
			// final frame.
			_, nextOK := readFrameHeaderAt(r, candidate+int64(h.frameLen))
			if !nextOK {
				if end, serr := r.Seek(0, io.SeekEnd); serr == nil && candidate+int64(h.frameLen) >= end {
					return candidate, h, nil
				}
				continue
			}
			return candidate, h, nil
		}
		if n < blockSize {
			return 0, frameHeader{}, errors.New("no valid mp3 frame found")
		}
		// Overlap by 3 bytes so a header straddling the block boundary is seen.
		pos += int64(n - 3)
	}
}

// Trailer marker sizes. An MP3 can carry metadata AFTER the audio as well as
// before it; those bytes are not audio and must be excluded from any byte-range
// arithmetic, or the duration inflates by the trailer's size.
const (
	id3v1Len       = 128 // "TAG"        + 125 bytes
	id3v1ExtLen    = 227 // "TAG+"       + 223 bytes, sits immediately before ID3v1
	apeFooterLen   = 32  // "APETAGEX"   + version/size/count/flags/reserved
	lyrics3End     = 9   // "LYRICS200", preceded by a 6-digit ASCII size
	lyrics3SizeLen = 6
	// trailerWindow is the tail slice examined per iteration. It comfortably
	// spans the longest fixed marker (the 227-byte ID3v1 extended tag) plus the
	// markers that can follow it.
	trailerWindow = 512
	// maxAPETagLen caps how much a single APE footer may peel. "APETAGEX" can
	// occur in entropy-coded audio payload by chance, and the size field beside
	// such a match is arbitrary, so an unbounded peel could silently delete
	// minutes of real audio. This is far above any legitimate APE tag, which
	// holds text metadata and perhaps a cover image.
	maxAPETagLen = 16 << 20
)

// audioEndOffset returns where audio stops: peeled is end of stream minus any
// ID3v1, ID3v1 extended, APE or Lyrics3v2 trailer, and raw is the unmodified end
// of stream.
//
// Trailers stack -- a file commonly ends Lyrics3v2, APE, ID3v1 in that order --
// so the scan loops from the end, peeling one marker per pass, until the tail no
// longer matches anything. Cost is a bounded read of the last few hundred bytes,
// which the CBR probe below needs anyway to know the audio byte range.
//
// BOTH ends are returned because a marker can occur in entropy-coded audio
// payload by chance, and this function cannot tell a genuine trailer from a
// coincidence: peeling a false match would silently delete real audio. The
// caller decides, by testing which candidate is consistent with the frame grid.
func audioEndOffset(r io.ReadSeeker) (peeled, raw int64, err error) {
	end, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, 0, err
	}
	raw = end

	buf := make([]byte, trailerWindow)
	for {
		n := int64(trailerWindow)
		if n > end {
			n = end
		}
		if n < 8 {
			return end, raw, nil
		}
		if _, serr := r.Seek(end-n, io.SeekStart); serr != nil {
			return 0, 0, serr
		}
		tail := buf[:n]
		if _, err := io.ReadFull(r, tail); err != nil {
			// The tail could not be read, so no trailer can be identified.
			// Fall back to the raw end rather than failing the whole parse.
			return end, raw, nil
		}

		if next, ok := peelTrailer(tail, end); ok {
			end = next
			continue
		}
		return end, raw, nil
	}
}

// peelTrailer identifies at most one trailing metadata block at the end of tail
// (whose last byte is at absolute offset end-1) and returns the new end offset.
// ok is false when nothing is recognized, which terminates the peel loop.
func peelTrailer(tail []byte, end int64) (int64, bool) {
	n := int64(len(tail))

	// ID3v1: exactly 128 bytes starting with "TAG" (but not "TAG+", which is the
	// extended tag and is handled as its own case below).
	//
	// "TAG" alone is only three bytes and MPEG payload is entropy-coded, so it
	// turns up in real audio by chance. The tag's text fields must therefore also
	// look like text, or a coincidence peels 128 bytes of genuine audio and the
	// duration comes out short.
	if n >= id3v1Len {
		s := tail[n-id3v1Len:]
		if string(s[0:3]) == "TAG" && s[3] != '+' && looksLikeID3v1Text(s[3:125]) {
			return end - id3v1Len, true
		}
	}
	// ID3v1 extended: 227 bytes starting with "TAG+". It normally precedes the
	// ID3v1 tag, so the loop reaches it on a later pass.
	if n >= id3v1ExtLen {
		s := tail[n-id3v1ExtLen:]
		if string(s[0:4]) == "TAG+" && looksLikeID3v1Text(s[4:184]) {
			return end - id3v1ExtLen, true
		}
	}
	// APE tag: a 32-byte footer whose size field covers the tag body and the
	// footer itself. Bit 31 of the flags means a 32-byte header precedes it too.
	if n >= apeFooterLen {
		f := tail[n-apeFooterLen:]
		if string(f[0:8]) == "APETAGEX" {
			size := int64(binary.LittleEndian.Uint32(f[12:16]))
			flags := binary.LittleEndian.Uint32(f[20:24])
			total := size
			if flags&0x80000000 != 0 {
				total += apeFooterLen
			}
			// Reject a size that is degenerate or larger than the stream: a
			// corrupt field must not push the audio end below zero.
			//
			// Also cap it. "APETAGEX" can occur in audio payload by chance, and
			// the size field beside it is then arbitrary, so an unbounded peel
			// could silently delete minutes of real audio. A real APE tag holds
			// text metadata and optionally a cover image; maxAPETagLen is far
			// above any legitimate one and far below a length worth worrying
			// about at 8 kbps.
			if total >= apeFooterLen && total <= end && total <= maxAPETagLen {
				return end - total, true
			}
		}
	}
	// Lyrics3v2: ends with "LYRICS200" preceded by a 6-digit ASCII byte count
	// covering the tag from "LYRICSBEGIN" up to but excluding those 15 bytes.
	if n >= lyrics3End+lyrics3SizeLen {
		if string(tail[n-lyrics3End:]) == "LYRICS200" {
			digits := tail[n-lyrics3End-lyrics3SizeLen : n-lyrics3End]
			size, ok := parseASCIIDigits(digits)
			if ok {
				total := size + lyrics3End + lyrics3SizeLen
				if total <= end {
					return end - total, true
				}
			}
		}
	}
	return end, false
}

// looksLikeID3v1Text reports whether b is plausibly an ID3v1 text region: its
// title/artist/album/comment fields, which are space- or NUL-padded printable
// text, not arbitrary bytes.
//
// The check exists because "TAG" and "TAG+" are short enough to appear in
// entropy-coded audio payload by chance, and an unvalidated match peels real
// audio and shortens the reported duration. Control bytes and 0xFF (an MPEG sync
// byte, which never appears in ID3v1 text) are the discriminator; high bytes are
// allowed, since ID3v1 has no defined encoding and non-Latin tags are common.
func looksLikeID3v1Text(b []byte) bool {
	for _, c := range b {
		if c == 0x00 || c >= 0x20 {
			// NUL padding and printable/extended bytes are all expected.
			if c != 0xFF {
				continue
			}
			return false
		}
		// A control byte below 0x20. Only the whitespace a tagger might leave is
		// plausible; anything else says this is not text.
		if c != '\t' && c != '\n' && c != '\r' {
			return false
		}
	}
	return true
}

// parseASCIIDigits converts a run of ASCII decimal digits to an int64. ok is
// false if any byte is not a digit, so a random byte run that happens to sit
// before a "LYRICS200"-looking string is not acted on.
func parseASCIIDigits(b []byte) (int64, bool) {
	var v int64
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, false
		}
		v = v*10 + int64(c-'0')
	}
	return v, true
}

// Constant-bitrate probe tuning. Together these bound the probe's I/O to
// probeCount+2 windows, each a small multiple of ONE frame length -- so the cost
// scales with the bitrate, never with the file size.
const (
	// probeCount is how many points are sampled between the head and the tail.
	probeCount = 5
	// probeChain is how many consecutive frames one probe must decode, all
	// agreeing, before that probe counts as confirming.
	probeChain = 4
	// headChainFrames is how many consecutive frames are checked from the first
	// frame onward. A variable-bitrate stream varies within a handful of frames,
	// so this run alone rejects essentially all of them, and it needs no resync
	// because the first frame's offset is already known exactly.
	headChainFrames = 16
	// gridSlackBytes is how far a probe's observed frame boundary may sit, IN
	// BYTES, from the position a constant bitrate predicts before the stream is
	// rejected.
	//
	// It must be a small BYTE count, never a multiple of the frame length. The
	// prediction rounds to the nearest whole frame, so a residual is at most half
	// a frame by construction -- a slack of even one frame length would make the
	// test vacuously true for every offset. In a genuine CBR stream the boundary
	// tracks the ideal grid to within the padding byte, so a few bytes is ample,
	// and the slack doubles as the bound on the largest anomaly that can hide.
	gridSlackBytes = 4
)

// windowFor returns the byte window needed to chain want frames of roughly
// refLen bytes. The slack covers the padding byte a CBR frame may carry, the
// final header's 4 bytes, and -- when the window starts at an arbitrary offset
// rather than a frame boundary -- one extra frame to resync within.
func windowFor(refLen, want int, resync bool) int {
	if refLen < minFrameLen {
		refLen = minFrameLen
	}
	frames := want
	if resync {
		frames++
	}
	return frames*(refLen+1) + 4
}

// undeclaredDuration derives the duration of a stream that declares no frame
// count, choosing between an O(1) size division and an exhaustive frame walk.
//
// Size division is EXACT for a constant-bitrate stream: audio bytes times 8
// divided by the bitrate is the definition of constant bitrate, and it is
// immune to the padding bit, which changes individual frame LENGTHS but not the
// bitrate they encode -- CBR encoders alternate padding precisely so the average
// frame length lands on the nominal rate. It is unboundedly wrong for a variable
// bitrate stream, so it is used only when sampling finds no bitrate variation.
//
// The fallback is the frame walk, which is exact for both but reads the whole
// audio range. Every inconclusive outcome -- an unreadable probe, a resync
// failure, a stream too short to sample -- selects the walk, so the cheap path
// is taken only on positive evidence.
func undeclaredDuration(r io.ReadSeeker, first frameHeader, firstPos int64) (float64, error) {
	peeledEnd, rawEnd, err := audioEndOffset(r)
	if err != nil {
		return 0, err
	}
	if rawEnd <= firstPos {
		return 0, errors.New("no mp3 audio data")
	}

	// A trailer marker can occur in entropy-coded audio payload by chance, and
	// peeling on a false match deletes real audio and shortens the duration.
	// Rather than guessing from the marker's contents whether it is genuine,
	// TEST it: in a constant-bitrate stream the audio ends on the frame grid, so
	// a peel that cut into audio leaves the end off-grid. Prefer the peeled end,
	// fall back to the raw one, and use size division only for a candidate that
	// both probes as constant AND ends where the grid predicts.
	bpf := bytesPerFrame(first)
	minProbeable := int64(windowFor(first.frameLen, headChainFrames, false)) +
		int64(windowFor(first.frameLen, probeChain, true))*(probeCount+1)

	for _, end := range [2]int64{peeledEnd, rawEnd} {
		audioBytes := end - firstPos
		// Sample only when there is more audio than the probe would itself read.
		// Below that the walk reads no more, and it is exact, so probing could
		// only lose.
		if audioBytes < minProbeable {
			continue
		}
		if !onFrameGrid(audioBytes, bpf) {
			continue
		}
		if !probeConstantBitRate(r, first, firstPos, end) {
			continue
		}
		// bitRate is in kbps (1000 bps), so bytes*8/(kbps*1000) is seconds.
		return float64(audioBytes) * 8 / float64(first.bitRate*1000), nil
	}

	// No candidate earned the cheap path. Walk, from the peeled end: the walk
	// stops at the first bytes that do not decode, so an over-peel costs it
	// nothing and an under-peel would let it wander into a trailer.
	return walkFrameChain(r, firstPos, peeledEnd)
}

// bytesPerFrame is the exact, fractional frame length a header's nominal bitrate
// implies. The integer frameLen is this rounded down, which is why a CBR encoder
// pads: the average frame length must land on this value.
func bytesPerFrame(h frameHeader) float64 {
	if h.sampleRate <= 0 {
		return 0
	}
	return float64(h.samplesPerFrame) * float64(h.bitRate*1000) / (8 * float64(h.sampleRate))
}

// onFrameGrid reports whether a byte offset measured from the first frame falls
// on a frame boundary that a constant bitrate predicts, within gridSlackBytes.
func onFrameGrid(offset int64, bpf float64) bool {
	if bpf <= 0 {
		return false
	}
	k := math.Round(float64(offset) / bpf)
	if k < 0 {
		return false
	}
	return math.Abs(float64(offset)-k*bpf) <= gridSlackBytes
}

// probeConstantBitRate reports whether the stream from firstPos to audioEnd is
// constant bitrate, sampling a bounded number of frame headers to decide.
//
// It compares BITRATE, never frame length: a CBR stream's frame lengths differ
// by one byte as the padding bit toggles, and treating that as variation would
// send every padded CBR file down the expensive path.
//
// Three kinds of evidence are required, and the third is what makes a bounded
// sample sufficient rather than merely suggestive:
//
//  1. A run of consecutive frames from the first frame, followed by frame length
//     so no resync is needed. A variable stream varies within a few frames and
//     is rejected here at no extra I/O.
//
//  2. Probes spread across the audio, each resyncing inside a small window and
//     requiring a short agreeing chain, plus one anchored at the tail.
//
//  3. GRID ALIGNMENT. Sampling alone cannot bound the answer: any anomaly small
//     enough to fall between two probe points is invisible, so the size division
//     could be wrong by an unbounded amount with every probe agreeing. But in a
//     constant-bitrate stream every frame boundary is predictable -- frame k
//     begins at firstPos + round(k * bytesPerFrame) -- so ANY inserted region,
//     any run of differently-sized frames, and any non-audio splice ANYWHERE
//     before a probe shifts that probe's observed boundary off the predicted
//     grid by the size of the anomaly. Checking alignment therefore tests the
//     whole prefix up to each probe, not just the bytes in the window, which is
//     what closes the gaps between probes.
//
// The slack is gridSlackBytes BYTES, not frame lengths: the prediction rounds to
// the nearest whole frame, so a residual is at most half a frame by construction
// and a frame-sized slack would accept everything. A few bytes covers the padding
// wander, and it doubles as the bound on the largest anomaly that can hide.
//
// It returns false on anything it cannot confirm. A false negative costs the
// caller the frame walk it would have done anyway; a false positive would report
// a wrong duration, so the asymmetry is deliberate.
func probeConstantBitRate(r io.ReadSeeker, first frameHeader, firstPos, audioEnd int64) bool {
	headWin := windowFor(first.frameLen, headChainFrames, false)
	probeWin := windowFor(first.frameLen, probeChain, true)
	buf := make([]byte, max(headWin, probeWin))

	// Evidence 1: consecutive frames from the start of the audio, followed by
	// frame length so no resync is needed.
	if !chainAgreesAt(r, buf[:headWin], first, firstPos, audioEnd, headChainFrames, false, nil) {
		return false
	}

	bpf := bytesPerFrame(first)
	if bpf <= 0 {
		return false
	}

	// Evidence 2 and 3: probes spread across the audio, plus one anchored at the
	// very end, each required to land on the predicted grid.
	span := audioEnd - firstPos
	offsets := make([]int64, 0, probeCount+1)
	for i := 1; i <= probeCount; i++ {
		offsets = append(offsets, firstPos+span*int64(i)/int64(probeCount+1))
	}
	if tail := audioEnd - int64(probeWin); tail > firstPos {
		offsets = append(offsets, tail)
	}

	for _, off := range offsets {
		if off <= firstPos || off >= audioEnd {
			continue
		}
		// onGrid rejects a boundary the constant-bitrate model does not predict.
		// found is the absolute offset of the frame the probe actually synced to.
		onGrid := func(found int64) bool {
			return onFrameGrid(found-firstPos, bpf)
		}
		if !chainAgreesAt(r, buf[:probeWin], first, off, audioEnd, probeChain, true, onGrid) {
			return false
		}
	}
	return true
}

// chainAgreesAt reads one window at pos, sized by the caller, and verifies a
// chain of want frames all matching ref's bitrate, sample rate, version, layer.
//
// When resync is true, pos is an arbitrary offset rather than a known frame
// boundary, so the window is scanned for an offset that starts an agreeing
// chain. When it is false, pos must already be a frame boundary and the very
// first header there must decode.
//
// onGrid, when non-nil, additionally requires the offset the chain starts at to
// be a position the constant-bitrate model predicts. It is what stops a stray
// frame-shaped byte run inside the window from satisfying the probe, and what
// extends each probe's reach back over every byte before it.
//
// Reaching audioEnd mid-chain is success, not failure: the remaining frames do
// not exist, and everything seen so far agreed. That allowance requires the
// chain to have reached the true end of audio, not merely the end of a window
// that happens to sit there, so it is granted only to a chain that consumed the
// window right up to audioEnd.
func chainAgreesAt(r io.ReadSeeker, buf []byte, ref frameHeader, pos, audioEnd int64, want int, resync bool, onGrid func(int64) bool) bool {
	if _, err := r.Seek(pos, io.SeekStart); err != nil {
		return false
	}
	n, err := io.ReadFull(r, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return false
	}
	// Never read past the end of audio into a trailer.
	if limit := audioEnd - pos; int64(n) > limit {
		n = int(limit)
	}
	if n < 4 {
		return false
	}
	window := buf[:n]

	starts := []int{0}
	if resync {
		starts = starts[:0]
		for i := 0; i+4 <= n; i++ {
			if window[i] != 0xFF {
				continue
			}
			starts = append(starts, i)
		}
	}

	for _, start := range starts {
		if onGrid != nil && !onGrid(pos+int64(start)) {
			continue
		}
		if chainFrom(window, start, ref, want) {
			return true
		}
	}
	return false
}

// chainFrom walks want frames from offset start within window, requiring each to
// decode and to agree with ref. Running out of window is failure, never success.
//
// An earlier revision accepted a short chain that ran off the end of the audio,
// so a probe whose window ends at audioEnd -- which the tail probe's always does
// -- was satisfied by a SINGLE agreeing header. Measured against random bytes
// that made the tail probe strictly weaker than the others (30 acceptances per
// 200,000 windows, versus 0 mid-stream), which is backwards: the tail is where a
// concatenated or re-encoded stream is most likely to differ.
//
// No allowance is needed. undeclaredDuration only probes a stream longer than
// minProbeable, and windowFor sizes every window to hold want frames plus a
// spare for resync, so a full chain always fits at every probe point.
func chainFrom(window []byte, start int, ref frameHeader, want int) bool {
	off := start
	for i := 0; i < want; i++ {
		if off+4 > len(window) {
			return false
		}
		h, ok := parseFrameHeader([4]byte{window[off], window[off+1], window[off+2], window[off+3]})
		if !ok || !sameFormat(h, ref) {
			return false
		}
		off += h.frameLen
	}
	return true
}

// sameFormat reports whether two frame headers describe the same constant
// stream. Padding and the CRC bit are deliberately not compared: both legally
// vary frame to frame within a single CBR stream.
func sameFormat(a, b frameHeader) bool {
	return a.bitRate == b.bitRate &&
		a.sampleRate == b.sampleRate &&
		a.mpegVer == b.mpegVer &&
		a.layer == b.layer
}

// Mp3 Calculate mp3 files duration.
//
// I/O cost depends on what the stream declares.
//
//   - Xing or VBRI header present: the frame count is read from it and only the
//     first frame's region is touched.
//   - Neither header present, constant bitrate: a bounded set of frame headers
//     is sampled across the stream (see probeConstantBitRate) and the duration
//     comes from dividing the audio byte range by the bitrate, which is exact
//     for CBR. Cost is a fixed handful of small windows regardless of file size.
//   - Neither header present, variable bitrate: the whole stream is read to end
//     of audio in 256 KiB blocks to count frames, because nothing in the file
//     declares the length and no cheaper method is correct. Only 4-byte frame
//     headers are decoded, never audio, but the bytes are still read.
//
// The last case is negligible for a local file but changes the cost profile for
// an io.ReadSeeker backed by the network, such as an HTTP range-request reader:
// size timeouts and any caching accordingly.
func Mp3(r io.ReadSeeker) (float64, error) {
	var duration float64

	// Jump over EVERY ID3v2 tag before really dealing with audio data.
	tagEnd, err := skipID3v2Tags(r)
	if err != nil {
		return 0, err
	}

	// Find the first byte offset that holds a genuine, chaining frame header.
	firstFramePos, hdr, err := findFirstFrame(r, tagEnd)
	if err != nil {
		return 0, err
	}

	mpegVer := hdr.mpegVer
	layer := hdr.layer
	sampleRate := hdr.sampleRate
	samplesPerFrame := hdr.samplesPerFrame

	// Position the reader just past the 4-byte header, which is where the
	// original single-pass scan left it and what the CRC / side-info /
	// Xing / VBRI bookkeeping below assumes.
	if _, err := r.Seek(firstFramePos+4, io.SeekStart); err != nil {
		return 0, err
	}

	// Jump 16-bit CRC after the 4 bytes MPEG header, if has
	if hdr.protection == 0 {
		if _, err := r.Seek(2, io.SeekCurrent); err != nil {
			return 0, err
		}
	}
	// Jump side info bytes
	if layer == layerIII {
		if _, err := r.Seek(getSideInfoLen(mpegVer, hdr.mode), io.SeekCurrent); err != nil {
			return 0, err
		}
	}

	var totalFrame uint32

	buf4 := make([]byte, 4)
	if _, err = io.ReadFull(r, buf4); err != nil {
		// Too few bytes remain after the side info for a Xing/VBRI marker, so
		// the stream declares no frame count. findFirstFrame already validated
		// a real frame at firstFramePos, so the audio is countable even though
		// the metadata slot is not there -- count it rather than failing. This
		// is reachable on short MPEG-2/2.5 frames and on a truncated tail.
		return undeclaredDuration(r, hdr, firstFramePos)
	}
	switch string(buf4) {
	case "VBRI":
		v, err := parseVBRI(r)
		if err != nil {
			return 0, err
		}
		totalFrame = v.totalFrame
	case "Xing", "Info":
		x, err := parseXing(r)
		if err != nil {
			return 0, err
		}
		totalFrame = x.totalFrame
	default:
		// No Xing/VBRI header, so the frame count is declared nowhere and has to
		// be derived from the audio itself.
		return undeclaredDuration(r, hdr, firstFramePos)
	}

	duration = (float64(samplesPerFrame) / float64(sampleRate)) * float64(totalFrame)
	return duration, nil
}

// walkFrameChain sums samplesPerFrame/sampleRate over every frame from startPos
// to end of stream, which is the exact duration of a stream that declares no
// frame count. It reads only frame headers -- it never decodes audio -- so the
// cost is one seek-and-read per frame, not a decode.
//
// Frames are validated through the same parseFrameHeader the first-frame scan
// uses, so both agree on what a frame is. On hitting bytes that do not decode,
// the walk rescans forward for the next real frame rather than aborting: a
// stream with garbage spliced mid-file still yields a duration for the audio
// that is actually there. A frame whose length would not advance the cursor is
// treated as end of stream, so a malformed file cannot spin here forever.
func walkFrameChain(r io.ReadSeeker, startPos, end int64) (float64, error) {
	// Read the stream in blocks: per-frame Seek+Read syscalls dominate the cost
	// otherwise, and a header is only 4 bytes.
	const blockSize = 256 * 1024
	block := make([]byte, blockSize)
	var blockStart, blockLen int64 = -1, 0

	// headerAt returns the 4 header bytes at absolute offset p, refilling the
	// block window when p falls outside it.
	headerAt := func(p int64) ([4]byte, bool) {
		var b [4]byte
		if p+4 > end {
			return b, false
		}
		if blockStart < 0 || p < blockStart || p+4 > blockStart+blockLen {
			if _, serr := r.Seek(p, io.SeekStart); serr != nil {
				return b, false
			}
			n, rerr := io.ReadFull(r, block)
			if n < 4 && rerr != nil {
				return b, false
			}
			blockStart, blockLen = p, int64(n)
		}
		off := p - blockStart
		copy(b[:], block[off:off+4])
		return b, true
	}

	var seconds float64
	pos := startPos
	for pos < end {
		b, ok := headerAt(pos)
		if !ok {
			break
		}
		h, valid := parseFrameHeader(b)
		if !valid {
			// Not a frame here. Scan forward for the next one rather than
			// abandoning the rest of the stream.
			next, _, ferr := findFirstFrame(r, pos+1)
			if ferr != nil {
				break
			}
			// findFirstFrame moved the reader, so the block window is stale.
			blockStart, blockLen = -1, 0
			pos = next
			continue
		}
		if h.frameLen <= 0 {
			break
		}
		seconds += float64(h.samplesPerFrame) / float64(h.sampleRate)
		pos += int64(h.frameLen)
	}
	return seconds, nil
}
