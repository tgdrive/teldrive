package files

import (
	"encoding/json"
	"os"

	"github.com/divyam234/teldrive/types"
	"github.com/go-jose/go-jose/v3"
)

func Encode(payload *types.JWTClaimsFileSharing) (string, error) {

	rcpt := jose.Recipient{
		Algorithm: jose.PBES2_HS256_A128KW,
		Key:       os.Getenv("JWT_SECRET"),
	}

	enc, err := jose.NewEncrypter(jose.A128CBC_HS256, rcpt, nil)

	if err != nil {
		return "", err
	}

	jwt, _ := json.Marshal(payload)

	jweObject, err := enc.Encrypt(jwt)

	if err != nil {
		return "", err
	}

	jweToken, err := jweObject.CompactSerialize()

	if err != nil {
		return "", err
	}
	return jweToken, nil
}

func Decode(token string) (*types.JWTClaimsFileSharing, error) {
	jwe, err := jose.ParseEncrypted(token)
	if err != nil {
		return nil, err
	}

	decryptedData, err := jwe.Decrypt(os.Getenv("JWT_SECRET"))

	if err != nil {
		return nil, err
	}

	jwtToken := &types.JWTClaimsFileSharing{}

	err = json.Unmarshal(decryptedData, jwtToken)

	if err != nil {
		return nil, err
	}

	return jwtToken, nil

}
