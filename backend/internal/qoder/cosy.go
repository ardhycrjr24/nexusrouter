package qoder

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CosyCreds carries the credential fields needed for COSY signing.
type CosyCreds struct {
	UserID    string
	AuthToken string
	Name      string
	Email     string
	MachineID string
}

// rsaPublicKey is parsed once at init time from RSAPublicKeyPEM.
var rsaPublicKey *rsa.PublicKey

func init() {
	block, _ := pem.Decode([]byte(RSAPublicKeyPEM))
	if block == nil {
		panic("qoder: failed to parse RSA public key PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		panic(fmt.Sprintf("qoder: failed to parse RSA public key: %v", err))
	}
	rsaPublicKey = pub.(*rsa.PublicKey)
}

// generateAESKey returns the first 16 characters of a fresh UUID, matching the
// JS implementation which uses uuidv4().slice(0, 16).
func generateAESKey() string {
	return uuid.NewString()[:16]
}

// pkcs7Pad pads data to a multiple of blockSize using PKCS#7 padding.
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padded := make([]byte, len(data)+padding)
	copy(padded, data)
	for i := len(data); i < len(padded); i++ {
		padded[i] = byte(padding)
	}
	return padded
}

// aesEncryptCBCBase64 encrypts plaintext with AES-128-CBC using keyStr as both
// the key and IV (first 16 bytes), applies PKCS7 padding, and returns the
// result base64-encoded.
func aesEncryptCBCBase64(plaintext string, keyStr string) (string, error) {
	keyBytes := []byte(keyStr)
	if len(keyBytes) != 16 {
		return "", fmt.Errorf("qoder: AES key must be 16 bytes, got %d", len(keyBytes))
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", fmt.Errorf("qoder: AES cipher: %w", err)
	}

	// IV reuses the key bytes (matches JS: keyBytes.subarray(0, 16)).
	iv := keyBytes[:16]
	padded := pkcs7Pad([]byte(plaintext), aes.BlockSize)

	mode := cipher.NewCBCEncrypter(block, iv)
	encrypted := make([]byte, len(padded))
	mode.CryptBlocks(encrypted, padded)

	return base64.StdEncoding.EncodeToString(encrypted), nil
}

// rsaEncryptBase64 encrypts data with the Qoder RSA public key using PKCS1
// padding and returns the result base64-encoded.
func rsaEncryptBase64(data string) (string, error) {
	encrypted, err := rsa.EncryptPKCS1v15(rand.Reader, rsaPublicKey, []byte(data))
	if err != nil {
		return "", fmt.Errorf("qoder: RSA encrypt: %w", err)
	}
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

// userInfo is the JSON structure AES-encrypted into the COSY payload.
type userInfo struct {
	UID           string `json:"uid"`
	SecurityToken string `json:"security_oauth_token"`
	Name          string `json:"name"`
	AID           string `json:"aid"`
	Email         string `json:"email"`
}

// encryptUserInfo AES-encrypts the user info JSON and RSA-encrypts the AES key.
func encryptUserInfo(info userInfo) (cosyKey, infoB64 string, err error) {
	aesKey := generateAESKey()
	plaintext, err := json.Marshal(info)
	if err != nil {
		return "", "", fmt.Errorf("qoder: marshal user info: %w", err)
	}

	infoB64, err = aesEncryptCBCBase64(string(plaintext), aesKey)
	if err != nil {
		return "", "", err
	}

	cosyKey, err = rsaEncryptBase64(aesKey)
	if err != nil {
		return "", "", err
	}

	return cosyKey, infoB64, nil
}

// md5Hex returns the lowercase hex MD5 digest of input.
func md5Hex(input []byte) string {
	sum := md5.Sum(input)
	return fmt.Sprintf("%x", sum)
}

// computeSigPath strips the leading "/algo" prefix from the request URL's path.
// Matches qodercli convention.
func computeSigPath(requestURL string) string {
	u, err := url.Parse(requestURL)
	if err != nil {
		return ""
	}
	pathname := u.Path
	if strings.HasPrefix(pathname, "/algo") {
		return pathname[len("/algo"):]
	}
	return pathname
}

// cosyPayload is the JSON structure base64-encoded into the COSY Authorization
// header.
type cosyPayload struct {
	Version     string `json:"version"`
	RequestID   string `json:"requestId"`
	Info        string `json:"info"`
	CosyVersion string `json:"cosyVersion"`
	IDEVersion  string `json:"ideVersion"`
}

// BuildCosyHeaders generates the full COSY header set for a single Qoder
// request. body is the exact bytes that will be sent (the encoded body when
// using &Encode=1). requestURL is the full request URL (used for sigPath).
func BuildCosyHeaders(body []byte, requestURL string, creds CosyCreds) (map[string]string, error) {
	if creds.UserID == "" {
		return nil, fmt.Errorf("qoder: user id is empty")
	}
	if creds.AuthToken == "" {
		return nil, fmt.Errorf("qoder: auth token is empty")
	}

	cosyKey, infoB64, err := encryptUserInfo(userInfo{
		UID:           creds.UserID,
		SecurityToken: creds.AuthToken,
		Name:          creds.Name,
		AID:           "",
		Email:         creds.Email,
	})
	if err != nil {
		return nil, err
	}

	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	requestID := uuid.NewString()

	payload := cosyPayload{
		Version:     "v1",
		RequestID:   requestID,
		Info:        infoB64,
		CosyVersion: IDEVersion,
		IDEVersion:  "",
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("qoder: marshal payload: %w", err)
	}
	payloadB64 := base64.StdEncoding.EncodeToString(payloadJSON)

	sigPath := computeSigPath(requestURL)

	// sig = md5Hex(payloadB64 + "\n" + cosyKey + "\n" + timestamp + "\n" + bodyLatin1 + "\n" + sigPath)
	sigInput := payloadB64 + "\n" + cosyKey + "\n" + timestamp + "\n" + string(body) + "\n" + sigPath
	sig := md5Hex([]byte(sigInput))

	machineID := creds.MachineID
	if machineID == "" {
		machineID = uuid.NewString()
	}
	bodyHash := md5Hex(body)
	bodyLength := fmt.Sprintf("%d", len(body))

	return map[string]string{
		"Authorization":          "Bearer COSY." + payloadB64 + "." + sig,
		"Cosy-Key":               cosyKey,
		"Cosy-User":              creds.UserID,
		"Cosy-Date":              timestamp,
		"Cosy-Version":           IDEVersion,
		"Cosy-Machineid":         machineID,
		"Cosy-Machinetoken":      machineID,
		"Cosy-Machinetype":       MachineType,
		"Cosy-Machineos":         MachineOS,
		"Cosy-Clienttype":        ClientType,
		"Cosy-Clientip":          "127.0.0.1",
		"Cosy-Bodyhash":          bodyHash,
		"Cosy-Bodylength":        bodyLength,
		"Cosy-Sigpath":           sigPath,
		"Cosy-Data-Policy":       DataPolicy,
		"Cosy-Organization-Id":   "",
		"Cosy-Organization-Tags": "",
		"Login-Version":          LoginVersion,
		"X-Request-Id":           uuid.NewString(),
	}, nil
}
