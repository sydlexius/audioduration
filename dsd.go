package audioduration

import (
	"encoding/binary"
	"errors"
	"io"
)

// DSD format specification
// https://dsd-guide.com/sites/default/files/white-papers/DSFFileFormatSpec_E.pdf

type dsdChunk struct {
	header        string
	chunkSize     uint64
	totalFileSize uint64
	metadatePtr   uint64
}

type fmtChunk struct {
	header         string
	chunkSize      uint64
	formatVer      uint32
	formatID       uint32
	channelType    uint32
	channelNum     uint32
	sampleFreq     uint32
	bitPerSec      uint32
	sampleCount    uint64
	blockSizePerCh uint32
	// reserved       uint32
}

// DSD Calculate dsd files duration.
func DSD(r io.ReadSeeker) (float64, error) {
	var duration float64 = 0
	var err error
	var dc dsdChunk
	var fc fmtChunk
	buf4 := make([]byte, 4)
	buf8 := make([]byte, 8)
	_, err = io.ReadFull(r, buf4)
	if err != nil {
		return 0, err
	}
	dc.header = string(buf4)
	if dc.header != "DSD " {
		return 0, errors.New("not valid dsd file")
	}
	_, err = io.ReadFull(r, buf8)
	if err != nil {
		return 0, err
	}
	dc.chunkSize = binary.LittleEndian.Uint64(buf8)
	if dc.chunkSize < 12 {
		return 0, errors.New("invalid DSD chunk size")
	}
	if _, err := r.Seek(int64(dc.chunkSize-12), io.SeekCurrent); err != nil {
		return 0, err
	}
	_, err = io.ReadFull(r, buf4)
	if err != nil {
		return 0, err
	}
	fc.header = string(buf4)
	if fc.header != "fmt " {
		return 0, errors.New("not valid dsd file")
	}
	_, err = io.ReadFull(r, buf8)
	if err != nil {
		return 0, err
	}
	for i := 0; i < 4; i++ {
		_, err = io.ReadFull(r, buf4)
		if err != nil {
			return 0, err
		}
	}
	_, err = io.ReadFull(r, buf4)
	if err != nil {
		return 0, err
	}
	fc.sampleFreq = binary.LittleEndian.Uint32(buf4)
	_, err = io.ReadFull(r, buf4)
	if err != nil {
		return 0, err
	}
	_, err = io.ReadFull(r, buf8)
	if err != nil {
		return 0, err
	}
	fc.sampleCount = binary.LittleEndian.Uint64(buf8)
	duration = float64(fc.sampleCount) / float64(fc.sampleFreq)
	return duration, nil
}
