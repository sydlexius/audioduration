package audioduration

import (
	"errors"
	"io"
)

// AAC parses raw AAC ADTS streams and returns duration in seconds.
// It scans ADTS frames, accumulating samples and dividing by sample rate.
// Ref: ISO/IEC 13818-7 (ADTS header)
func AAC(r io.ReadSeeker) (float64, error) {
	buf := make([]byte, 10)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return 0, err
	}

	if string(buf[0:4]) == "ADIF" {
		if _, err := r.Seek(4, io.SeekStart); err != nil {
			return 0, err
		}
		return parseADIF(r)
	}

	if string(buf[0:3]) == "ID3" {
		offset := parseID3v2Length(buf)
		if _, err := r.Seek(offset, io.SeekCurrent); err != nil {
			return 0, err
		}
	} else {
		// rewind if not ID3
		if _, err := r.Seek(0, io.SeekStart); err != nil {
			return 0, err
		}
	}

	// Sampling frequencies per ADTS sampling_frequency_index
	sampleRates := []int{
		96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050,
		16000, 12000, 11025, 8000, 7350,
	}

	var sampleRate int = 0
	var totalFrame = 0

	// Find first sync word (0xFFF)
	if err := aacSeekNextSync(r); err != nil {
		return 0, err
	}

	for {
		hdr := make([]byte, 7)
		_, err := io.ReadFull(r, hdr)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return 0, err
		}

		// Validate sync
		if !(hdr[0] == 0xFF && (hdr[1]&0xF0) == 0xF0) {
			// try to resync from next byte
			if _, err := r.Seek(-6, io.SeekCurrent); err != nil {
				return 0, err
			}
			if err := aacSeekNextSync(r); err != nil {
				if err == io.EOF || err == io.ErrUnexpectedEOF {
					break
				}
				return 0, err
			}
			continue
		}

		//protectionAbsent := hdr[1] & 0x01
		//profile := (hdr[2] >> 6) & 0x03 // not used currently
		sfIndex := (hdr[2] >> 2) & 0x0F
		if int(sfIndex) >= len(sampleRates) {
			return 0, errors.New("invalid AAC sampling frequency index")
		}
		if sampleRate == 0 {
			sampleRate = sampleRates[sfIndex]
		}
		// channel configuration could be parsed if needed:
		// chCfg := ((hdr[2]&0x01)<<2) | ((hdr[3] >> 6) & 0x03)

		// aac_frame_length is 13 bits across hdr[3:6]
		frameLen := int((uint32(hdr[3]&0x03) << 11) | (uint32(hdr[4]) << 3) | (uint32(hdr[5]) >> 5))
		if frameLen <= 7 {
			return 0, errors.New("invalid AAC frame length")
		}

		// number_of_raw_data_blocks_in_frame (2 bits) at hdr[6] low 2 bits
		nrdb := int(hdr[6] & 0x03)
		totalFrame += (1 + nrdb)

		headLen := 7
		//if protectionAbsent == 0 {
		// read CRC (2 bytes) which are part of the header but not in hdr
		//	crc := make([]byte, 2)
		//	if _, err := io.ReadFull(r, crc); err != nil {
		//		return 0, err
		//	}
		//	headLen = 9
		//}
		// Skip rest of frame payload
		payloadLen := int64(frameLen - headLen)
		if payloadLen > 0 {
			if _, err := r.Seek(payloadLen, io.SeekCurrent); err != nil {
				return 0, err
			}
		}
	}

	if sampleRate == 0 {
		return 0, errors.New("could not determine AAC sample rate")
	}

	return float64(totalFrame) * 1024 / float64(sampleRate), nil
}

// aacSeekNextSync advances the reader until an ADTS syncword (0xFFF) is found
func aacSeekNextSync(r io.ReadSeeker) error {
	buf := make([]byte, 1)
	preHead := false
	for {
		_, err := io.ReadFull(r, buf)
		if err != nil {
			return err
		}
		if preHead && (buf[0]&0xF0) == 0xF0 {
			// rewind 2 bytes so caller can read full header
			if _, err := r.Seek(-2, io.SeekCurrent); err != nil {
				return err
			}
			return nil
		} else {
			preHead = false
		}

		if buf[0] == 0xFF {
			preHead = true
		}
	}
}

func parseADIF(r io.ReadSeeker) (float64, error) {
	return 0, errors.New("ADIF format not implemented")
}
