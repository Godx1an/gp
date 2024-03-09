package jwt

import (
	"errors"
	"github.com/dgrijalva/jwt-go"
	"time"
)

type Jwt struct {
	Info string
	jwt.StandardClaims
}

const (
	TokenExpireDuration          = time.Hour * 24 * 7
	TokenExpireDurationAutoLogin = time.Hour * 24 * 30
)

var Secret = []byte("Cephalon-User-Center")

func GenerateJwt(str string, autoLogin ...bool) (string, error) {
	expireTime := TokenExpireDuration
	for _, auto := range autoLogin {
		if auto {
			expireTime = TokenExpireDurationAutoLogin
		}
	}
	var token *jwt.Token
	index := Jwt{
		str,
		jwt.StandardClaims{
			ExpiresAt: time.Now().Add(expireTime).Unix(),
		},
	}
	token = jwt.NewWithClaims(jwt.SigningMethodHS256, index)
	return token.SignedString(Secret)
}

func ParseToken(tokenString string) (*Jwt, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Jwt{}, func(token *jwt.Token) (interface{}, error) {
		return Secret, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Jwt); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("INVALID TOKEN")
}
