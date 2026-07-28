package audioduration

import (
	"encoding/binary"
	"errors"
	"io"
)

// https://xiph.org/flac/format.html#metadata_block_streaminfo

// FLAC Calculate flac files duration.
func FLAC(r io.ReadSeeker) (float64, error) {
	buf := make([]byte, 4)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return 0, err
	}
	hdr := string(buf)
	if hdr != "fLaC" {
		return 0, errors.New("expected 'fLaC' at file start")
	}
	var duration float64 = 0
	for {
		_, err = io.ReadFull(r, buf)
		if err != nil {
			break
		}
		blockType := buf[0] & 0x7f
		blockSize := binary.BigEndian.Uint32(buf) & 0x00FFFFFF
		if blockType == 0 { // Metadata block type is Streaminfo
			// Streaminfo block must be at least 18 bytes
			if blockSize < 18 {
				return 0, errors.New("invalid FLAC streaminfo block size")
			}
			streamInfoBuf := make([]byte, blockSize)
			_, err = io.ReadFull(r, streamInfoBuf)
			if err != nil {
				break
			}
			sampleRate := binary.BigEndian.Uint32(
				append([]byte{0}, streamInfoBuf[10:13]...)) >> 4
			totalSamples := binary.BigEndian.Uint64(
				append([]byte{0, 0, 0}, streamInfoBuf[13:18]...)) & 0x7ffffffff
			duration = float64(totalSamples) / float64(sampleRate)
			break
		} else {
			return 0, errors.New("unexpected block type")
		}
	}
	return duration, err
}
