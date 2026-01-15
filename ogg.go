package audioduration

import (
	"bytes"
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
	if oph.headerType>>2 == 1 {
		return true
	}
	return false
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
	binary.Read(bytes.NewReader(buf[9:13]), binary.LittleEndian, &vih.bitrateMax)
	binary.Read(bytes.NewReader(buf[13:17]), binary.LittleEndian, &vih.bitrateNom)
	binary.Read(bytes.NewReader(buf[17:21]), binary.LittleEndian, &vih.bitrateMin)
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

// getOggBitrate Get bitrate of OGG file. Reserved.
func getOggBitrate(vih vorbisIdentHdr) int32 {
	var bitrate int32
	if vih.bitrateMax == 0 && vih.bitrateMin == 0 && vih.bitrateNom != 0 {
		bitrate = vih.bitrateNom
	}
	if vih.bitrateMax == vih.bitrateMin && vih.bitrateMin == vih.bitrateNom {
		bitrate = vih.bitrateNom
	}
	if vih.bitrateNom == 0 {
		bitrate = (vih.bitrateMax + vih.bitrateMin) / 2
	}
	return bitrate
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
			vih, err = parseIdentHdr(r)
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
