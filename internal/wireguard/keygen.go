package wireguard

import (
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// GeneratePrivateKey generates a new WireGuard private key and returns it
// as a base64-encoded string.
func GeneratePrivateKey() (string, error) {
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return "", err
	}
	return key.String(), nil
}

// PublicKey derives the public key from a base64-encoded WireGuard private key.
func PublicKey(privateKey string) (string, error) {
	key, err := wgtypes.ParseKey(privateKey)
	if err != nil {
		return "", err
	}
	return key.PublicKey().String(), nil
}

// GeneratePresharedKey generates a new WireGuard preshared key and returns it
// as a base64-encoded string.
func GeneratePresharedKey() (string, error) {
	key, err := wgtypes.GenerateKey()
	if err != nil {
		return "", err
	}
	return key.String(), nil
}
