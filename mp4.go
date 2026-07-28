package audioduration

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
)

// The specification of MP4 file could be get here.
// https://developer.apple.com/library/archive/documentation/QuickTime/QTFF/QTFFChap2/qtff2.html

// MP4 format has a hierarchy structure. The basic unit is Atom. Different type
// of Atom has different structure. And Atom could contain other types of Atom.
// The duration information could be accessed at:
// moov.trak.mdia.mdhd
// which structure is difined at:
// https://developer.apple.com/library/archive/documentation/QuickTime/QTFF/QTFFChap2/qtff2.html#//apple_ref/doc/uid/TP40000939-CH204-SW34

// Mp4 Calculate mp4 files duration.
func Mp4(r io.ReadSeeker) (float64, error) {
	for {
		typ, size, headerLen, err := readAtomHeader(r)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return 0, err
		}
		// size comes from the file, so a crafted atom controls it fully. The
		// first test rejects an underflowing subtraction; the second rejects a
		// value that would wrap negative in the int64 conversion below and seek
		// backwards.
		if size < headerLen || size-headerLen > math.MaxInt64 {
			return 0, errors.New("invalid MP4 atom size")
		}
		content := int64(size - headerLen) //nolint:gosec // reason: the guard above rejects the underflow (size < headerLen) and every value whose int64 conversion would wrap (> MaxInt64), so this is provably in range; G115 cannot track the bound across the subtraction.

		switch typ {
		case "moov":
			tmp, _ := r.Seek(0, io.SeekCurrent)
			moovEnd := tmp + content
			var movieTimeScale uint64 = 0

			for {
				childTyp, childSize, childHdrLen, err := readAtomHeader(r)
				if err != nil {
					return 0, err
				}
				// Same reasoning as the parent atom above: reject both the
				// underflowing subtraction and the value that would wrap
				// negative in the int64 conversion.
				if childSize < childHdrLen || childSize-childHdrLen > math.MaxInt64 {
					return 0, errors.New("invalid MP4 child atom size")
				}
				// compute end of this child box to realign after parsing its content
				tmp, _ = r.Seek(0, io.SeekCurrent)
				childEnd := tmp + int64(childSize-childHdrLen) //nolint:gosec // reason: the guard above rejects the underflow (childSize < childHdrLen) and every value whose int64 conversion would wrap (> MaxInt64), so this is provably in range; G115 cannot track the bound across the subtraction.

				switch childTyp {
				case "mvhd":
					// We only use mvhd to get movie timescale for elst conversion
					ts, _, err := parseMvhd(r)
					if err != nil {
						return 0, err
					}
					movieTimeScale = ts
					// seek to end of mvhd box
					if _, err := r.Seek(childEnd, io.SeekStart); err != nil {
						return 0, err
					}
				case "trak":
					if d, ok, err := readAudioDurationInTrak(r, childEnd, movieTimeScale); err != nil {
						return 0, err
					} else if ok {
						return d, nil
					} else {
						// ensure aligned at end of trak
						if _, err := r.Seek(childEnd, io.SeekStart); err != nil {
							return 0, err
						}
					}
				default:
					// skip unknown child box
					if _, err := r.Seek(childEnd, io.SeekStart); err != nil {
						return 0, err
					}
				}

				if childEnd >= moovEnd {
					break
				}
			}
		default:
			if err := skip(r, content); err != nil {
				return 0, err
			}
		}
	}

	return 0, errors.New("audio mdhd not found")
}

func readAudioDurationInTrak(r io.ReadSeeker, endPos int64, movieTS uint64) (float64, bool, error) {
	var dur float64 = 0
	var elstSecs float64 = 0
	var haveMdhd bool
	var haveElst bool

	for {
		typ, size, headerLen, err := readAtomHeader(r)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return 0, false, err
		}
		if size < headerLen || size-headerLen > math.MaxInt64 {
			return 0, false, errors.New("invalid MP4 atom size in trak")
		}
		content := int64(size - headerLen) //nolint:gosec // reason: the guard above rejects the underflow (size < headerLen) and every value whose int64 conversion would wrap (> MaxInt64), so this is provably in range; G115 cannot track the bound across the subtraction.
		tmp, _ := r.Seek(0, io.SeekCurrent)
		childEnd := tmp + content

		switch typ {
		case "mdia":
			isAudio, mts, md, ok, err := readMdiaInfo(r, childEnd)
			if err != nil {
				return 0, false, err
			}
			if ok && isAudio {
				if mts != 0 {
					haveMdhd = true
					dur = float64(md) / float64(mts)
				}
			}
		case "edts":
			if movieTS != 0 {
				if secs, ok, err := readElstSeconds(r, childEnd, movieTS); err != nil {
					return 0, false, err
				} else if ok {
					haveElst = true
					elstSecs = secs
				}
			} else {
				if err := skip(r, content); err != nil {
					return 0, false, err
				}
			}
		default:
			if err := skip(r, content); err != nil {
				return 0, false, err
			}
		}

		if childEnd >= endPos {
			break
		}
	}

	if haveElst && elstSecs > 0 {
		return elstSecs, true, nil
	}
	if haveMdhd {
		return dur, true, nil
	}
	if _, err := r.Seek(endPos, io.SeekStart); err != nil {
		return 0, false, err
	}
	return 0, false, nil
}

func readMdiaInfo(r io.ReadSeeker, endPos int64) (isAudio bool, ts uint64, dur uint64, hasMdhd bool, err error) {
	isAudio = false
	hasMdhd = false
	ts = 0
	dur = 0

	for {
		typ, size, headerLen, e := readAtomHeader(r)
		if e != nil {
			if e == io.EOF || e == io.ErrUnexpectedEOF {
				break
			}
			err = e
			return
		}
		if size < headerLen || size-headerLen > math.MaxInt64 {
			err = errors.New("invalid MP4 atom size in mdia")
			return
		}
		content := int64(size - headerLen) //nolint:gosec // reason: the guard above rejects the underflow (size < headerLen) and every value whose int64 conversion would wrap (> MaxInt64), so this is provably in range; G115 cannot track the bound across the subtraction.
		tmp, _ := r.Seek(0, io.SeekCurrent)
		childEnd := tmp + content

		switch typ {
		case "hdlr":
			if content < 12 {
				err = errors.New("invalid MP4 atom size in mdia hdlr")
				return
			}
			buf := make([]byte, 12)
			if _, e := io.ReadFull(r, buf); e != nil {
				err = e
				return
			}
			handler := string(buf[8:12])
			if handler == "soun" {
				isAudio = true
			}
			if e := skip(r, content-12); e != nil {
				err = e
				return
			}
		case "mdhd":
			vbuf := make([]byte, 4)
			if _, e := io.ReadFull(r, vbuf); e != nil {
				err = e
				return
			}
			version := vbuf[0]
			if version == 1 {
				// creation(8) + modification(8)
				if e := skip(r, 16); e != nil {
					err = e
					return
				}
				b4 := make([]byte, 4)
				if _, e := io.ReadFull(r, b4); e != nil {
					err = e
					return
				}
				ts = uint64(binary.BigEndian.Uint32(b4))
				b8 := make([]byte, 8)
				if _, e := io.ReadFull(r, b8); e != nil {
					err = e
					return
				}
				dur = binary.BigEndian.Uint64(b8)
			} else {
				// version 0: creation(4) + modification(4)
				if e := skip(r, 8); e != nil {
					err = e
					return
				}
				b4 := make([]byte, 4)
				if _, e := io.ReadFull(r, b4); e != nil {
					err = e
					return
				}
				ts = uint64(binary.BigEndian.Uint32(b4))
				if _, e := io.ReadFull(r, b4); e != nil {
					err = e
					return
				}
				dur = uint64(binary.BigEndian.Uint32(b4))
			}

			if _, e := r.Seek(childEnd, io.SeekStart); e != nil {
				err = e
				return
			}
			hasMdhd = true
		default:
			if e := skip(r, content); e != nil {
				err = e
				return
			}
		}

		if childEnd >= endPos {
			break
		}
	}
	return
}

func readElstSeconds(r io.ReadSeeker, endPos int64, movieTS uint64) (float64, bool, error) {
	for {
		typ, size, headerLen, err := readAtomHeader(r)
		if err != nil {
			return 0, false, err
		}
		if size < headerLen || size-headerLen > math.MaxInt64 {
			err = errors.New("invalid MP4 atom size in edts")
			return 0, false, err
		}
		content := int64(size - headerLen) //nolint:gosec // reason: the guard above rejects the underflow (size < headerLen) and every value whose int64 conversion would wrap (> MaxInt64), so this is provably in range; G115 cannot track the bound across the subtraction.
		tmp, _ := r.Seek(0, io.SeekCurrent)
		childEnd := tmp + content

		switch typ {
		case "elst":
			b4 := make([]byte, 4)
			if _, err := io.ReadFull(r, b4); err != nil {
				return 0, false, err
			}
			version := b4[0]

			if _, err := io.ReadFull(r, b4); err != nil {
				return 0, false, err
			}
			entryCount := binary.BigEndian.Uint32(b4)

			var totalDur uint64 = 0

			for i := uint32(0); i < entryCount; i++ {
				var segDur uint64
				if version == 1 {
					b8 := make([]byte, 8)
					if _, err := io.ReadFull(r, b8); err != nil {
						return 0, false, err
					}
					segDur = binary.BigEndian.Uint64(b8)
					// media_time(8)
					if _, err := io.ReadFull(r, b8); err != nil {
						return 0, false, err
					}
				} else {
					if _, err := io.ReadFull(r, b4); err != nil {
						return 0, false, err
					}
					segDur = uint64(binary.BigEndian.Uint32(b4))
					// media_time(4)
					if _, err := io.ReadFull(r, b4); err != nil {
						return 0, false, err
					}
				}
				// media_rate_integer(2)+fraction(2)
				if _, err := io.ReadFull(r, b4); err != nil {
					return 0, false, err
				}
				// Only normal rate 1.0 is counted
				if binary.BigEndian.Uint16(b4[0:2]) == 1 {
					totalDur += segDur
				}
			}

			// skip remainder
			if _, err := r.Seek(childEnd, io.SeekStart); err != nil {
				return 0, false, err
			}

			if totalDur > 0 {
				return float64(totalDur) / float64(movieTS), true, nil
			}
			return 0, false, nil
		default:
			if err = skip(r, content); err != nil {
				return 0, false, err
			}
		}

		if childEnd >= endPos {
			break
		}
	}
	return 0, false, nil
}

func parseMvhd(r io.ReadSeeker) (uint64, uint64, error) {
	b4 := make([]byte, 4)
	if _, err := io.ReadFull(r, b4); err != nil {
		return 0, 0, err
	}
	version := b4[0]
	if version == 1 {
		if err := skip(r, 16); err != nil {
			return 0, 0, err
		}
		if _, err := io.ReadFull(r, b4); err != nil {
			return 0, 0, err
		}
		ts := uint64(binary.BigEndian.Uint32(b4))
		b8 := make([]byte, 8)
		if _, err := io.ReadFull(r, b8); err != nil {
			return 0, 0, err
		}
		dur := binary.BigEndian.Uint64(b8)
		return ts, dur, nil
	} else {
		if err := skip(r, 8); err != nil {
			return 0, 0, err
		}
		if _, err := io.ReadFull(r, b4); err != nil {
			return 0, 0, err
		}
		ts := uint64(binary.BigEndian.Uint32(b4))
		if _, err := io.ReadFull(r, b4); err != nil {
			return 0, 0, err
		}
		dur := uint64(binary.BigEndian.Uint32(b4))
		return ts, dur, nil
	}
}

func readAtomHeader(r io.ReadSeeker) (string, uint64, uint64, error) {
	b8 := make([]byte, 8)
	if _, err := io.ReadFull(r, b8); err != nil {
		return "", 0, 0, err
	}
	size := uint64(binary.BigEndian.Uint32(b8[0:4]))
	typ := string(b8[4:8])
	headerLen := uint64(8)
	if size == 1 {
		if _, err := io.ReadFull(r, b8); err != nil {
			return "", 0, 0, err
		}
		size = binary.BigEndian.Uint64(b8)
		headerLen = 16
	}
	return typ, size, headerLen, nil
}

func skip(r io.ReadSeeker, n int64) error {
	if n <= 0 {
		return nil
	}
	_, err := r.Seek(n, io.SeekCurrent)
	return err
}
