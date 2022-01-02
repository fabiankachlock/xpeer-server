package util

import (
	"crypto/rand"
	"encoding/base64"

	"github.com/fabiankachlock/xpeer-server/pkg/xpeer"
)

func FilterSliceByPeerId(slice []string, id string) []string {
	newSlice := make([]string, len(slice)-1)
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
		xpeer.ErrorLogger.Println(err.Error())
		return "<<<err-id-gen>>>" // return error indicating placeholder id
	}

	// return base64 encoded random bytes appended with server id
	return base64.RawURLEncoding.EncodeToString(b) + xpeer.SERVER_PREFIX_DIVIDER + xpeer.SERVER_SUFFIX
}
