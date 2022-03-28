// Copyright (c) 2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"encoding/binary"
	"encoding/json"
)

func bytesToStringMap(bytes []byte) (map[string]string, error) {
	if bytes == nil {
		return make(map[string]string), nil
	}

	var stringMap map[string]string
	err := json.Unmarshal(bytes, &stringMap)
	if err != nil {
		return nil, err
	}

	// stringMap can still be nil here, eg. if bytes == "null".
	if stringMap == nil {
		stringMap = make(map[string]string)
	}

	return stringMap, nil
}

func int64ToBytes(i int64) []byte {
	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes, uint64(i))
	return bytes
}

func bytesToInt64(bytes []byte) int64 {
	return int64(binary.LittleEndian.Uint64(bytes))
}

func uint32ToBytes(i uint32) []byte {
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, i)
	return bytes
}

func bytesToUint32(bytes []byte) uint32 {
	return binary.LittleEndian.Uint32(bytes)
}

func bytesToBool(bytes []byte) bool {
	return bytes[0] == 1
}

func boolToBytes(b bool) []byte {
	if b {
		return []byte{1}
	}

	return []byte{0}
}
