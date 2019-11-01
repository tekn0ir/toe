package iot

import (
	"io/ioutil"
	"log"
	"time"

	"github.com/dgrijalva/jwt-go"
)

// createJWT creates a Cloud IoT Core JWT for the given project id.
// algorithm can be one of ["RSA256", "ES256"].
func CreateJWTToken(projectID string, privateKeyPath string, expiration time.Duration) (string, error) {
	log.Println("[iot] Load Private Key")
	keyBytes, err := ioutil.ReadFile(privateKeyPath)
	if err != nil {
		return "", err
	}

	token := jwt.New(jwt.SigningMethodRS256)
	token.Claims = jwt.StandardClaims{
		Audience:  projectID,
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(expiration).Unix(),
	}

	log.Println("[iot] Parse Private Key")
	privKey, err := jwt.ParseRSAPrivateKeyFromPEM(keyBytes)
	if err != nil {
		return "", err
	}

	return token.SignedString(privKey)
}
