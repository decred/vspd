// Copyright (c) 2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
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
