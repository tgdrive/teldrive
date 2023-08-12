package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/divyam234/teldrive-go/types"
	"github.com/divyam234/teldrive-go/utils"
	"github.com/divyam234/teldrive-go/utils/auth"
	"github.com/gin-gonic/gin"
	"github.com/go-jose/go-jose/v3/jwt"
)

type AuthService struct {
	SessionMaxAge int
}

func IP4toInt(IPv4Address net.IP) int64 {
	IPv4Int := big.NewInt(0)
	IPv4Int.SetBytes(IPv4Address.To4())
	return IPv4Int.Int64()
}

func Pack32BinaryIP4(ip4Address string) []byte {
	ipv4Decimal := IP4toInt(net.ParseIP(ip4Address))

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uint32(ipv4Decimal))
	return buf.Bytes()
}

func generateTgSession(dcID int, serverAddress string, authKey []byte, port int) string {

	dcIDByte := byte(dcID)
	serverAddressBytes := Pack32BinaryIP4(serverAddress)
	portByte := make([]byte, 2)
	binary.BigEndian.PutUint16(portByte, uint16(port))

	packet := make([]byte, 0)
	packet = append(packet, dcIDByte)
	packet = append(packet, serverAddressBytes...)
	packet = append(packet, portByte...)
	packet = append(packet, authKey...)

	base64Encoded := base64.URLEncoding.EncodeToString(packet)
	return "1" + base64Encoded
}

func GetUserSessionCookieName(c *gin.Context) string {

	isHttps := c.Request.URL.Scheme == "https"
	var cookieName string
	if isHttps {
		cookieName = "__Secure-user-session"
	} else {
		cookieName = "user-session"
	}

	return cookieName
}

func (as *AuthService) LogIn(c *gin.Context) *types.AppError {
	dcMaps := map[int]string{
		1: "149.154.175.53",
		2: "149.154.167.51",
		3: "149.154.175.100",
		4: "149.154.167.91",
		5: "91.108.56.130",
	}
	var session types.TgSession
	if err := c.ShouldBindJSON(&session); err != nil {
		return &types.AppError{Error: errors.New("invalid request payload"), Code: http.StatusBadRequest}
	}

	now := time.Now().UTC()

	authBytes, _ := hex.DecodeString(session.AuthKey)

	sessionData := generateTgSession(session.DcID, dcMaps[session.DcID], authBytes, 443)

	jwtClaims := &types.JWTClaims{Claims: jwt.Claims{
		Subject:  session.UserID,
		IssuedAt: jwt.NewNumericDate(now),
		Expiry:   jwt.NewNumericDate(now.Add(time.Duration(as.SessionMaxAge) * time.Second)),
	}, TgSession: sessionData, Name: session.Name, UserName: session.UserName, Bot: session.Bot, IsPremium: session.IsPremium}

	jweToken, err := auth.Encode(jwtClaims)

	if err != nil {
		return &types.AppError{Error: err, Code: http.StatusBadRequest}
	}
	c.SetCookie(GetUserSessionCookieName(c), jweToken, as.SessionMaxAge, "/", c.Request.Host, false, false)
	return nil
}

func (as *AuthService) GetSession(c *gin.Context) *types.Session {

	cookie, err := c.Request.Cookie(GetUserSessionCookieName(c))

	if err != nil {
		return nil
	}

	jwePayload, err := auth.Decode(cookie.Value)

	if err != nil {
		return nil
	}

	now := time.Now().UTC()

	newExpires := now.Add(time.Duration(as.SessionMaxAge) * time.Second)

	session := &types.Session{Name: jwePayload.Name, UserName: jwePayload.UserName, Expires: newExpires.Format(time.RFC3339)}

	jwePayload.IssuedAt = jwt.NewNumericDate(now)

	jwePayload.Expiry = jwt.NewNumericDate(newExpires)

	jweToken, err := auth.Encode(jwePayload)

	if err != nil {
		return nil
	}
	c.SetCookie(GetUserSessionCookieName(c), jweToken, as.SessionMaxAge, "/", c.Request.Host, false, false)
	return session
}

func (as *AuthService) Logout(c *gin.Context) *types.AppError {
	val, _ := c.Get("jwtUser")
	jwtUser := val.(*types.JWTClaims)
	userId, _ := strconv.Atoi(jwtUser.Subject)
	tgClient, err, stop := utils.GetAuthClient(jwtUser.TgSession, userId)

	if err != nil {
		return &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}

	_, err = tgClient.Tg.API().AuthLogOut(context.Background())
	if err != nil {
		return &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}
	utils.StopClient(stop, userId)
	c.SetCookie(GetUserSessionCookieName(c), "", -1, "/", c.Request.Host, false, false)
	return nil
}
