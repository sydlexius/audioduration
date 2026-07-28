package audioduration

import (
	"encoding/binary"
	"errors"
	"io"
)

type mp3Hdr uint32

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

	layerIdx := 0
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

func mpegVerStr(mpegVer uint8) string {
	mpegVerTable := map[uint8]string{
		mpeg1:  "MPEG-1",
		mpeg2:  "MPEG-2",
		mpeg25: "MPEG-2.5",
	}
	return mpegVerTable[mpegVer]
}

func layerStr(layer uint8) string {
	layerTable := map[uint8]string{
		layerI:   "Layer I",
		layerII:  "Layer II",
		layerIII: "Layer III",
	}
	return layerTable[layer]
}

func modeStr(mode uint8) string {
	var modeStr string
	switch mode {
	case 0b00:
		modeStr = "Stereo"
	case 0b01:
		modeStr = "Joint stereo"
	case 0b10:
		modeStr = "Dual channel"
	case 0b11:
		modeStr = "Single channel"
	}
	return modeStr
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

// minFrameLen is a sanity floor for a decoded frame length. The smallest legal
// MPEG frame is a 32 kbps MPEG-2.5 Layer III frame at 8000 Hz, which is 288
// bytes; 24 is a deliberately conservative floor that rejects the degenerate
// lengths a false sync produces without risking a legitimate frame.
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

// Mp3 Calculate mp3 files duration.
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
	frameLen := hdr.frameLen

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
		return 0, err
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
		fSize, err := r.Seek(0, io.SeekEnd)
		if err != nil {
			return 0, err
		}
		audioDataSize := fSize - firstFramePos
		totalFrame = uint32(audioDataSize / int64(frameLen))
	}

	duration = (float64(samplesPerFrame) / float64(sampleRate)) * float64(totalFrame)
	return duration, nil
}
