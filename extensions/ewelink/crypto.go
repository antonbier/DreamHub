package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

// eWeLink's LAN protocol with "LAN control" on (encrypt=true): AES-128-CBC
// with PKCS#7 padding, the key being MD5 of the device's devicekey (from
// the owner's eWeLink account), the IV sent alongside, all base64.

var errNoKey = errors.New("no device key configured for this device")

func aesKey(deviceKey string) []byte {
	sum := md5.Sum([]byte(deviceKey))
	return sum[:]
}

// DecryptTXT turns an announcement's data1…data4 into the device's state
// as JSON (e.g. {"switch":"on"}).
func DecryptTXT(txt map[string]string, deviceKey string) (map[string]any, error) {
	if txt["encrypt"] != "true" {
		// DIY mode: plain JSON, split the same way.
		return parseState([]byte(joinData(txt)))
	}
	if deviceKey == "" {
		return nil, errNoKey
	}
	iv, err := base64.StdEncoding.DecodeString(txt["iv"])
	if err != nil || len(iv) != aes.BlockSize {
		return nil, errors.New("bad iv")
	}
	ct, err := base64.StdEncoding.DecodeString(joinData(txt))
	if err != nil {
		return nil, err
	}
	plain, err := decrypt(aesKey(deviceKey), iv, ct)
	if err != nil {
		return nil, err
	}
	return parseState(plain)
}

func joinData(txt map[string]string) string {
	var b strings.Builder
	for _, k := range []string{"data1", "data2", "data3", "data4"} {
		b.WriteString(txt[k])
	}
	return b.String()
}

func parseState(b []byte) (map[string]any, error) {
	var state map[string]any
	if err := json.Unmarshal(b, &state); err != nil {
		return nil, errors.New("the device's state isn't readable — is the device key right?")
	}
	return state, nil
}

func decrypt(key, iv, ct []byte) ([]byte, error) {
	if len(ct) == 0 || len(ct)%aes.BlockSize != 0 {
		return nil, errors.New("ciphertext isn't a whole number of blocks")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, ct)
	pad := int(out[len(out)-1])
	if pad == 0 || pad > aes.BlockSize || pad > len(out) || !bytes.Equal(out[len(out)-pad:], bytes.Repeat([]byte{byte(pad)}, pad)) {
		return nil, errors.New("bad padding — is the device key right?")
	}
	return out[:len(out)-pad], nil
}

// Encrypt returns base64 IV and ciphertext for a command payload.
func Encrypt(deviceKey string, plain []byte) (iv64, data64 string, err error) {
	if deviceKey == "" {
		return "", "", errNoKey
	}
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return "", "", err
	}
	block, err := aes.NewCipher(aesKey(deviceKey))
	if err != nil {
		return "", "", err
	}
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	padded := append(append([]byte(nil), plain...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	ct := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, padded)
	return base64.StdEncoding.EncodeToString(iv), base64.StdEncoding.EncodeToString(ct), nil
}
