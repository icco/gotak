package sanic

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
)

func IntToBytes(i int64) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.LittleEndian, i)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func IntToString(i int64, totalBits uint64) (string, error) {
	bts, err := IntToBytes(i)
	if err != nil {
		return "", err
	}
	bts = RemoveUnusedBytes(bts, totalBits)
	str := RemoveSixTrailingZeroBits(base64.RawURLEncoding.EncodeToString(bts),
		totalBits)
	return str, nil
}

func RemoveUnusedBytes(bts []byte, totalBits uint64) []byte {
	bytesLen := totalBits / 8
	if totalBits%8 != 0 {
		bytesLen++
	}
	return bts[:bytesLen]
}

func RemoveSixTrailingZeroBits(s string, totalBits uint64) string {
	strLen := int(totalBits / 6)
	if len(s) == strLen+1 {
		return s[:strLen]
	}
	return s
}
