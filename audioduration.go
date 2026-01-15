package audioduration

import (
	"fmt"
	"io"
)

// File Type Constant
const (
	TypeFlac int = 0
	TypeMp4  int = 1
	TypeMp3  int = 2
	TypeOgg  int = 3
	TypeDsd  int = 4
	TypeWav  int = 5
	TypeAac  int = 6
	TypeWebM int = 7
)

// Duration Get duration of specific music file type.
func Duration(file io.ReadSeeker, filetype int) (float64, error) {
	var d float64
	var err error
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	switch filetype {
	case TypeFlac:
		d, err = FLAC(file)
	case TypeMp4:
		d, err = Mp4(file)
	case TypeMp3:
		d, err = Mp3(file)
	case TypeOgg:
		d, err = Ogg(file)
	case TypeDsd:
		d, err = DSD(file)
	case TypeWav:
		d, err = Wav(file)
	case TypeAac:
		d, err = AAC(file)
	case TypeWebM:
		d, err = WebM(file)
	default:
		err = fmt.Errorf("unsupported type: %d", filetype)
	}
	file.Seek(0, io.SeekStart)
	return d, err
}
