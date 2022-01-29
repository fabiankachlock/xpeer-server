package util

import (
	"crypto/rand"
	"encoding/base64"
)

// return given slice without specified string (id)
func FilterSliceByPeerId(slice []string, id string) []string {
	newLen := len(slice) - 1
	if newLen <= 0 {
		return []string{}
	}

	newSlice := make([]string, newLen)
	for _, elm := range slice {
		if elm != id {
			newSlice = append(newSlice, elm)
		}
	}
	return newSlice
}

const (
	RAW_ID_LENGTH = 12 // results in a 16 char base64 encoded string
)

// generate a secure unique random id in valid xpeer-spec format
func GenerateId() string {
	b := make([]byte, RAW_ID_LENGTH)

	_, err := rand.Read(b) // generate secure random bytes
	if err != nil {
		//xpeer.ErrorLogger.Println(err.Error())
		return "<<<err-id-gen>>>" // return error indicating placeholder id
	}

	// return base64 encoded random bytes appended with server id
	return base64.RawURLEncoding.EncodeToString(b)
}
