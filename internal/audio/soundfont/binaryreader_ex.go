package soundfont

import (
	"errors"
	"io"
)

func readFixedLengthString(r io.Reader, length int32) (string, error) {
	var data = make([]byte, length)
	n, err := r.Read(data)
	if err != nil {
		return "", err
	}
	if n != int(length) {
		return "", errors.New("failed to read the fixed-length string")
	}

	var actualLength int32
	for actualLength = 0; actualLength < length; actualLength++ {
		if data[actualLength] == 0 {
			break
		}
	}

	return string(data[0:actualLength]), nil
}
