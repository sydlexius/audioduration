package audioduration

import (
	"encoding/binary"
	"io"
)

// oggPageHead The struct for ogg page header
// https://xiph.org/ogg/doc/framing.html#page_header
type oggPageHead struct {
	pattern     string
	version     uint8
	headerType  uint8
	granulePos  uint64
	bitstreamSN uint32
	pageSeqNum  uint32
	checksum    uint32
	pageSegs    uint8
	segTable    []uint8
}

func (oph oggPageHead) IsLastPage() bool {
	return oph.headerType>>2 == 1
}

const identHdr = "\x01vorbis"

// vorbisIdentHdr The struct for vorbis identification header.
// https://xiph.org/vorbis/doc/Vorbis_I_spec.html#x1-610004.2
type vorbisIdentHdr struct {
	vorbisVersion   uint32
	audioChannels   uint8
	audioSampleRate uint32
	bitrateMax      int32
	bitrateNom      int32
	bitrateMin      int32
	blocksize0      uint8
	blocksize1      uint8
	framingFlag     uint8
}

// readInt32LE decodes 4 little-endian bytes as a SIGNED 32-bit integer. The
// Vorbis bitrate fields are signed and a negative value is legal -- it means the
// field is unset -- so the full uint32 range must map onto the full int32 range.
// The conversion is a deliberate reinterpretation of the same 32 bits, not a
// narrowing one, so it cannot lose information and G115 does not apply.
//
//nolint:gosec // reason: intentional signed reinterpretation of a fixed-width field, not a narrowing conversion; the Vorbis spec defines these bitrates as signed.
func readInt32LE(b []byte) int32 {
	return int32(binary.LittleEndian.Uint32(b))
}

func parseIdentHdr(r io.ReadSeeker) (vorbisIdentHdr, error) {
	var vih vorbisIdentHdr
	buf := make([]byte, 23)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return vih, err
	}
	vih.vorbisVersion = binary.LittleEndian.Uint32(buf[0:4])
	vih.audioChannels = buf[4]
	vih.audioSampleRate = binary.LittleEndian.Uint32(buf[5:9])
	// These replace three binary.Read calls whose errors were discarded: buf is
	// already in memory and exactly 23 bytes, so they could not fail, but
	// decoding the integer directly makes that structural rather than assumed.
	vih.bitrateMax = readInt32LE(buf[9:13])
	vih.bitrateNom = readInt32LE(buf[13:17])
	vih.bitrateMin = readInt32LE(buf[17:21])
	// Confused while reading sepecification whether blocksize_0 and blocksize_1
	// is little endian or not, and which one occurs first. So just assume it
	// according to the sample file's situation. It is processed as pattern below.
	// [blocksize_1] [blocksize_0]
	// |   4-bits   |   4-bits   |
	// Both is treated as big endian. Then the condition that [blocksize_0] must
	// less than or equal to [blocksize_1] can be satisfied.
	// Anyway, this won't affect what we really need (just samplerate) here.
	vih.blocksize0 = buf[21] & 0x0F
	vih.blocksize1 = (buf[21] & 0xF0) >> 4
	vih.framingFlag = buf[22]
	if _, err := r.Seek(-23, io.SeekCurrent); err != nil {
		return vih, err
	}
	return vih, nil
}

// Ogg Calculate ogg files duration.
func Ogg(r io.ReadSeeker) (float64, error) {
	var err error
	var oggPH oggPageHead
	var vih vorbisIdentHdr
	var samples uint64
	var duration float64
	seg := make([]byte, 7)
Mainloop:
	for {
		headBuf := make([]byte, 27)
		_, err = io.ReadFull(r, headBuf)
		if err != nil {
			break
		}
		if string(headBuf[0:4]) != "OggS" {
			continue
		}
		oggPH.pattern = "OggS"
		oggPH.version = headBuf[4]
		oggPH.headerType = headBuf[5]
		oggPH.granulePos = binary.LittleEndian.Uint64(headBuf[6:14])
		oggPH.bitstreamSN = binary.LittleEndian.Uint32(headBuf[14:18])
		oggPH.pageSeqNum = binary.LittleEndian.Uint32(headBuf[18:22])
		oggPH.checksum = binary.LittleEndian.Uint32(headBuf[22:26])
		oggPH.pageSegs = headBuf[26]
		oggPH.segTable = []uint8{}
		var dataSegSize int64 = 0
		for i := uint8(0); i < oggPH.pageSegs; i++ {
			segTableItem := make([]byte, 1)
			_, err = io.ReadFull(r, segTableItem)
			if err != nil {
				break Mainloop
			}
			oggPH.segTable = append(oggPH.segTable, segTableItem[0])
			dataSegSize += int64(segTableItem[0])
		}
		if oggPH.IsLastPage() {
			samples = oggPH.granulePos
		}
		_, err = io.ReadFull(r, seg)
		if err != nil {
			break
		}
		if string(seg) == identHdr {
			// The error was previously assigned and then overwritten unread by
			// the Seek below, so a failure here silently left vih zeroed and
			// the final duration divided by a zero sample rate. Break like
			// every other read in this loop: a truncated stream yields io.EOF,
			// which the check after the loop treats as a normal end.
			if vih, err = parseIdentHdr(r); err != nil {
				break
			}
		}
		if _, err = r.Seek(-7, io.SeekCurrent); err != nil {
			break
		}
		if _, err = r.Seek(dataSegSize, io.SeekCurrent); err != nil {
			break
		}
	}
	if err != io.EOF {
		return 0, err
	}
	samplerate := vih.audioSampleRate
	duration = float64(samples) / float64(samplerate)
	return duration, nil
}
