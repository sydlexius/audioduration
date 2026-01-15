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

// Mp3 Calculate mp3 files duration.
func Mp3(r io.ReadSeeker) (float64, error) {
	buf := make([]byte, 1)
	id3v2headbuf := make([]byte, 10)
	var err error
	preHead := false
	var firstFrameStartPos uint32
	var duration float64

	// Jump over the ID3v2 tags before really deal with audio data.
	_, err = io.ReadFull(r, id3v2headbuf)
	if err != nil {
		return 0, err
	}
	if string(id3v2headbuf[0:3]) == "ID3" {
		id3v2offset := parseID3v2Length(id3v2headbuf)
		if _, err := r.Seek(id3v2offset, io.SeekCurrent); err != nil {
			return 0, err
		}
		firstFrameStartPos = uint32(len(id3v2headbuf)) + uint32(id3v2offset)
	} else {
		// no ID3v2 head
		if _, err := r.Seek(0, io.SeekStart); err != nil {
			return 0, err
		}
	}
	// Use loop to find pattern 1111 1111 111? ????
	for {
		_, err = io.ReadFull(r, buf)
		if err != nil {
			return 0, err
		}
		if preHead && (buf[0]>>5) == 0b111 {
			firstFrameStartPos--
			break
		} else {
			preHead = false
		}
		if buf[0] == 0xFF {
			preHead = true
		}
		firstFrameStartPos++
	}

	// 1111 1111, 111B BCCD, EEEE FFGH, IIJJ KLMM
	//                     ^
	//             buf[0]  |
	//                     fp
	//
	mpegVer := (buf[0] >> 3) & 0b00011
	layer := (buf[0] & 0b00000110) >> 1
	protection := (buf[0] & 0x1)

	_, err = io.ReadFull(r, buf)
	if err != nil {
		return 0, err
	}
	// 1111 1111, 111B BCCD, EEEE FFGH, IIJJ KLMM
	//                                ^
	//                        buf[0]  |
	//                                fp
	bitRateIndex := buf[0] >> 4
	bitRate := getBitRate(mpegVer, layer, bitRateIndex)
	if bitRate == 0 {
		return 0, errors.New("invalid bit rate")
	}
	sampleFreqIndex := (buf[0] >> 2) & 0b000011
	sampleRate := getSampleRate(mpegVer, sampleFreqIndex)
	if sampleRate == 0 {
		return 0, errors.New("invalid sample rate")
	}
	padding := (buf[0] >> 1) & 0b0000001
	samplesPerFrame := getSamplesPerFrame(mpegVer, layer)
	frameLen := frameLength(layer, padding, samplesPerFrame, bitRate, sampleRate)
	if frameLen == 0 {
		return 0, errors.New("invalid frame length")
	}

	_, err = io.ReadFull(r, buf)
	if err != nil {
		return 0, err
	}
	// 1111 1111, 111B BCCD, EEEE FFGH, IIJJ KLMM
	//                                           ^
	//                                   buf[0]  |
	//                                           fp
	mode := buf[0] >> 6

	// Jump 16-bit CRC after the 4 bytes MPEG header, if has
	if protection == 0 {
		if _, err := r.Seek(2, io.SeekCurrent); err != nil {
			return 0, err
		}
	}
	// Jump side info bytes
	if layer == layerIII {
		if _, err := r.Seek(getSideInfoLen(mpegVer, mode), io.SeekCurrent); err != nil {
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
		audioDataSize := fSize - int64(firstFrameStartPos)
		totalFrame = uint32(audioDataSize / int64(frameLen))
	}

	duration = (float64(samplesPerFrame) / float64(sampleRate)) * float64(totalFrame)
	return duration, nil
}
