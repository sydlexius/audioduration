package audioduration

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
)

// DSD format specification
// https://dsd-guide.com/sites/default/files/white-papers/DSFFileFormatSpec_E.pdf

// dsdChunk is the leading DSD chunk. Only the fields the duration calculation
// reads are kept; the rest of the chunk is seeked past. Full wire layout, in
// order: header(4) chunkSize(8) totalFileSize(8) metadataPtr(8).
type dsdChunk struct {
	header    string
	chunkSize uint64
}

// fmtChunk is the DSD format chunk. As above, only the read fields are kept.
// Full wire layout, in order: header(4) chunkSize(8) formatVer(4) formatID(4)
// channelType(4) channelNum(4) sampleFreq(4) bitPerSec(4) sampleCount(8)
// blockSizePerCh(4) reserved(4).
type fmtChunk struct {
	header      string
	sampleFreq  uint32
	sampleCount uint64
}

// DSD Calculate dsd files duration.
func DSD(r io.ReadSeeker) (float64, error) {
	var duration float64
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
	// chunkSize is read straight from the file, so a crafted input controls it
	// fully. Below 12 the subtraction underflows to a huge value; above
	// MaxInt64 the int64 conversion wraps negative and seeks BACKWARDS. Both
	// are rejected here rather than converted.
	if dc.chunkSize < 12 || dc.chunkSize-12 > math.MaxInt64 {
		return 0, errors.New("invalid DSD chunk size")
	}
	if _, err := r.Seek(int64(dc.chunkSize-12), io.SeekCurrent); err != nil { //nolint:gosec // reason: the guard above rejects the underflow (< 12) and every value whose int64 conversion would wrap (> MaxInt64), so this is provably in range; G115 cannot track the bound across the subtraction.
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
