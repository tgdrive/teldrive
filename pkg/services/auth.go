package services

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/divyam234/teldrive/config"
	"github.com/divyam234/teldrive/internal/auth"
	"github.com/divyam234/teldrive/internal/tgc"
	"github.com/divyam234/teldrive/internal/utils"
	"github.com/divyam234/teldrive/pkg/models"
	"github.com/divyam234/teldrive/pkg/schemas"
	"github.com/divyam234/teldrive/pkg/types"
	"github.com/gin-gonic/gin"
	"github.com/go-jose/go-jose/v3/jwt"
	"github.com/gorilla/websocket"
	"github.com/gotd/td/session"
	tgauth "github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type AuthService struct {
	Db                *gorm.DB
	SessionMaxAge     int
	SessionCookieName string
	log               *zap.Logger
}

func NewAuthService(db *gorm.DB, logger *zap.Logger) *AuthService {
	return &AuthService{
		Db:                db,
		SessionMaxAge:     30 * 24 * 60 * 60,
		SessionCookieName: "user-session",
		log:               logger.Named("auth")}
}

func ip4toInt(IPv4Address net.IP) int64 {
	IPv4Int := big.NewInt(0)
	IPv4Int.SetBytes(IPv4Address.To4())
	return IPv4Int.Int64()
}

func pack32BinaryIP4(ip4Address string) []byte {
	ipv4Decimal := ip4toInt(net.ParseIP(ip4Address))

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uint32(ipv4Decimal))
	return buf.Bytes()
}

func generateTgSession(dcID int, authKey []byte, port int) string {

	dcMaps := map[int]string{
		1: "149.154.175.53",
		2: "149.154.167.51",
		3: "149.154.175.100",
		4: "149.154.167.91",
		5: "91.108.56.130",
	}

	dcIDByte := byte(dcID)
	serverAddressBytes := pack32BinaryIP4(dcMaps[dcID])
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

func setCookie(c *gin.Context, key string, value string, age int) {

	if config.GetConfig().CookieSameSite {
		c.SetSameSite(2)
	} else {
		c.SetSameSite(4)
	}

	c.SetCookie(key, value, age, "/", "", config.GetConfig().Https, true)

}

func checkUserIsAllowed(userName string) bool {
	found := false
	allowedUsers := config.GetConfig().AllowedUsers
	if len(allowedUsers) > 0 {
		for _, user := range allowedUsers {
			if user == userName {
				found = true
				break
			}
		}
	} else {
		found = true
	}
	return found
}

func (as *AuthService) LogIn(c *gin.Context) (*schemas.Message, *types.AppError) {
	var session schemas.TgSession
	if err := c.ShouldBindJSON(&session); err != nil {
		return nil, &types.AppError{Error: errors.New("invalid request payload"), Code: http.StatusBadRequest}
	}

	if !checkUserIsAllowed(session.UserName) {
		return nil, &types.AppError{Error: errors.New("user not allowed"), Code: http.StatusUnauthorized}
	}

	now := time.Now().UTC()

	jwtClaims := &types.JWTClaims{Claims: jwt.Claims{
		Subject:  strconv.FormatInt(session.UserID, 10),
		IssuedAt: jwt.NewNumericDate(now),
		Expiry:   jwt.NewNumericDate(now.Add(time.Duration(as.SessionMaxAge) * time.Second)),
	}, TgSession: session.Sesssion,
		Name:      session.Name,
		UserName:  session.UserName,
		Bot:       session.Bot,
		IsPremium: session.IsPremium,
	}

	tokenhash := md5.Sum([]byte(session.Sesssion))
	hexToken := hex.EncodeToString(tokenhash[:])
	jwtClaims.Hash = hexToken

	jweToken, err := auth.Encode(jwtClaims)

	if err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusBadRequest}
	}

	user := models.User{
		UserId:    session.UserID,
		Name:      session.Name,
		UserName:  session.UserName,
		IsPremium: session.IsPremium,
	}

	var result []models.User

	if err := as.Db.Model(&models.User{}).Where("user_id = ?", session.UserID).
		Find(&result).Error; err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}
	if len(result) == 0 {
		if err := as.Db.Create(&user).Error; err != nil {
			return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
		}
		//Create root folder on first login

		file := &models.File{
			Name:     "root",
			Type:     "folder",
			MimeType: "drive/folder",
			Path:     "/",
			Depth:    utils.IntPointer(0),
			UserID:   session.UserID,
			Status:   "active",
			ParentID: "root",
		}
		if err := as.Db.Create(file).Error; err != nil {
			return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
		}
	}

	setCookie(c, as.SessionCookieName, jweToken, as.SessionMaxAge)

	//create session

	if err := as.Db.Create(&models.Session{UserId: session.UserID, Hash: hexToken, Session: session.Sesssion}).Error; err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}

	return &schemas.Message{Message: "login success"}, nil
}

func (as *AuthService) GetSession(c *gin.Context) *schemas.Session {

	cookie, err := c.Request.Cookie(as.SessionCookieName)

	if err != nil {
		return nil
	}

	jwePayload, err := auth.Decode(cookie.Value)

	if err != nil {
		return nil
	}

	now := time.Now().UTC()

	newExpires := now.Add(time.Duration(as.SessionMaxAge) * time.Second)

	session := &schemas.Session{Name: jwePayload.Name,
		UserName: jwePayload.UserName,
		Hash:     jwePayload.Hash,
		Expires:  newExpires.Format(time.RFC3339)}

	jwePayload.IssuedAt = jwt.NewNumericDate(now)

	jwePayload.Expiry = jwt.NewNumericDate(newExpires)

	jweToken, err := auth.Encode(jwePayload)

	if err != nil {
		return nil
	}
	setCookie(c, as.SessionCookieName, jweToken, as.SessionMaxAge)
	return session
}

func (as *AuthService) Logout(c *gin.Context) (*schemas.Message, *types.AppError) {
	val, _ := c.Get("jwtUser")
	jwtUser := val.(*types.JWTClaims)
	client, _ := tgc.UserLogin(c, jwtUser.TgSession)

	tgc.RunWithAuth(c, as.log, client, "", func(ctx context.Context) error {
		_, err := client.API().AuthLogOut(c)
		return err
	})

	setCookie(c, as.SessionCookieName, "", -1)
	as.Db.Where("session = ?", jwtUser.TgSession).Delete(&models.Session{})
	return &schemas.Message{Message: "logout success"}, nil
}

func prepareSession(user *tg.User, data *session.Data) *schemas.TgSession {
	sessionString := generateTgSession(data.DC, data.AuthKey, 443)
	session := &schemas.TgSession{
		Sesssion:  sessionString,
		UserID:    user.ID,
		Bot:       user.Bot,
		UserName:  user.Username,
		Name:      fmt.Sprintf("%s %s", user.FirstName, user.LastName),
		IsPremium: user.Premium,
	}
	return session
}

func (as *AuthService) HandleMultipleLogin(c *gin.Context) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	dispatcher := tg.NewUpdateDispatcher()
	loggedIn := qrlogin.OnLoginToken(dispatcher)
	sessionStorage := &session.StorageMemory{}
	tgClient := tgc.NoLogin(c, dispatcher, sessionStorage)

	err = tgClient.Run(c, func(ctx context.Context) error {
		for {
			message := &types.SocketMessage{}
			err := conn.ReadJSON(message)

			if err != nil {
				return err
			}
			if message.AuthType == "qr" {
				go func() {
					authorization, err := tgClient.QR().Auth(c, loggedIn, func(ctx context.Context, token qrlogin.Token) error {
						conn.WriteJSON(map[string]interface{}{"type": "auth", "payload": map[string]string{"token": token.URL()}})
						return nil
					})

					if tgerr.Is(err, "SESSION_PASSWORD_NEEDED") {
						conn.WriteJSON(map[string]interface{}{"type": "auth", "message": "2FA required"})
						return
					}

					if err != nil {
						conn.WriteJSON(map[string]interface{}{"type": "error", "message": err.Error()})
						return
					}
					user, ok := authorization.User.AsNotEmpty()
					if !ok {
						conn.WriteJSON(map[string]interface{}{"type": "error", "message": "auth failed"})
						return
					}
					if !checkUserIsAllowed(user.Username) {
						conn.WriteJSON(map[string]interface{}{"type": "error", "message": "user not allowed"})
						tgClient.API().AuthLogOut(c)
						return
					}
					res, _ := sessionStorage.LoadSession(c)
					sessionData := &types.SessionData{}
					json.Unmarshal(res, sessionData)
					session := prepareSession(user, &sessionData.Data)
					conn.WriteJSON(map[string]interface{}{"type": "auth", "payload": session, "message": "success"})
				}()
			}
			if message.AuthType == "phone" && message.Message == "sendcode" {
				go func() {
					res, err := tgClient.Auth().SendCode(c, message.PhoneNo, tgauth.SendCodeOptions{})
					if err != nil {
						conn.WriteJSON(map[string]interface{}{"type": "error", "message": err.Error()})
						return
					}
					code := res.(*tg.AuthSentCode)
					conn.WriteJSON(map[string]interface{}{"type": "auth", "payload": map[string]string{"phoneCodeHash": code.PhoneCodeHash}})
				}()
			}
			if message.AuthType == "phone" && message.Message == "signin" {
				go func() {
					auth, err := tgClient.Auth().SignIn(c, message.PhoneNo, message.PhoneCode, message.PhoneCodeHash)

					if errors.Is(err, tgauth.ErrPasswordAuthNeeded) {
						conn.WriteJSON(map[string]interface{}{"type": "auth", "message": "2FA required"})
						return
					}

					if err != nil {
						conn.WriteJSON(map[string]interface{}{"type": "error", "message": err.Error()})
						return
					}
					user, ok := auth.User.AsNotEmpty()
					if !ok {
						conn.WriteJSON(map[string]interface{}{"type": "error", "message": "auth failed"})
						return
					}
					if !checkUserIsAllowed(user.Username) {
						conn.WriteJSON(map[string]interface{}{"type": "error", "message": "user not allowed"})
						tgClient.API().AuthLogOut(c)
						return
					}
					res, _ := sessionStorage.LoadSession(c)
					sessionData := &types.SessionData{}
					json.Unmarshal(res, sessionData)
					session := prepareSession(user, &sessionData.Data)
					conn.WriteJSON(map[string]interface{}{"type": "auth", "payload": session, "message": "success"})
				}()
			}

			if message.AuthType == "2fa" && message.Password != "" {
				go func() {
					auth, err := tgClient.Auth().Password(c, message.Password)
					if err != nil {
						conn.WriteJSON(map[string]interface{}{"type": "error", "message": err.Error()})
						return
					}
					user, ok := auth.User.AsNotEmpty()
					if !ok {
						conn.WriteJSON(map[string]interface{}{"type": "error", "message": "auth failed"})
						return
					}
					if !checkUserIsAllowed(user.Username) {
						conn.WriteJSON(map[string]interface{}{"type": "error", "message": "user not allowed"})
						tgClient.API().AuthLogOut(c)
						return
					}
					res, _ := sessionStorage.LoadSession(c)
					sessionData := &types.SessionData{}
					json.Unmarshal(res, sessionData)
					session := prepareSession(user, &sessionData.Data)
					conn.WriteJSON(map[string]interface{}{"type": "auth", "payload": session, "message": "success"})
				}()
			}
		}
	})

	if err != nil {
		return
	}
}
