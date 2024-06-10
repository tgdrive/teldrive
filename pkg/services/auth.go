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

	"github.com/divyam234/teldrive/internal/auth"
	"github.com/divyam234/teldrive/internal/cache"
	"github.com/divyam234/teldrive/internal/config"
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
	"gorm.io/gorm"
)

type AuthService struct {
	db  *gorm.DB
	cnf *config.Config
}

func NewAuthService(db *gorm.DB, cnf *config.Config) *AuthService {
	return &AuthService{db: db, cnf: cnf}

}

func (as *AuthService) LogIn(c *gin.Context, session *schemas.TgSession) (*schemas.Message, *types.AppError) {

	if !checkUserIsAllowed(as.cnf.JWT.AllowedUsers, session.UserName) {
		return nil, &types.AppError{Error: errors.New("user not allowed"),
			Code: http.StatusUnauthorized}
	}

	now := time.Now().UTC()

	jwtClaims := &types.JWTClaims{Claims: jwt.Claims{
		Subject:  strconv.FormatInt(session.UserID, 10),
		IssuedAt: jwt.NewNumericDate(now),
		Expiry:   jwt.NewNumericDate(now.Add(as.cnf.JWT.SessionTime)),
	}, TgSession: session.Sesssion,
		Name:      session.Name,
		UserName:  session.UserName,
		Bot:       session.Bot,
		IsPremium: session.IsPremium,
	}

	tokenhash := md5.Sum([]byte(session.Sesssion))
	hexToken := hex.EncodeToString(tokenhash[:])
	jwtClaims.Hash = hexToken

	jweToken, err := auth.Encode(as.cnf.JWT.Secret, jwtClaims)

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

	if err := as.db.Model(&models.User{}).Where("user_id = ?", session.UserID).
		Find(&result).Error; err != nil {
		return nil, &types.AppError{Error: err}
	}
	if len(result) == 0 {
		if err := as.db.Create(&user).Error; err != nil {
			return nil, &types.AppError{Error: err}
		}

		//Create root folder on first login
		file := &models.File{
			Name:     "root",
			Type:     "folder",
			MimeType: "drive/folder",
			Depth:    utils.IntPointer(0),
			UserID:   session.UserID,
			Status:   "active",
			ParentID: "root",
		}
		if err := as.db.Create(file).Error; err != nil {
			return nil, &types.AppError{Error: err}
		}
	}

	client, _ := tgc.AuthClient(c, &as.cnf.TG, session.Sesssion)

	var auth *tg.Authorization

	err = client.Run(c, func(ctx context.Context) error {
		auths, err := client.API().AccountGetAuthorizations(c)
		if err != nil {
			return err
		}
		for _, a := range auths.Authorizations {
			if a.Current {
				auth = &a
				break
			}
		}
		return nil
	})

	if err != nil {
		return nil, &types.AppError{Error: err}

	}

	//create session
	if err := as.db.Create(&models.Session{UserId: session.UserID, Hash: hexToken,
		Session: session.Sesssion, AuthHash: GenAuthHash(auth)}).Error; err != nil {
		return nil, &types.AppError{Error: err}
	}

	setSessionCookie(c, jweToken, int(as.cnf.JWT.SessionTime.Seconds()))

	return &schemas.Message{Message: "login success"}, nil
}

func (as *AuthService) GetSession(c *gin.Context) *schemas.Session {

	cookie, err := c.Request.Cookie("user-session")

	if err != nil {
		return nil
	}

	jwePayload, err := auth.Decode(as.cnf.JWT.Secret, cookie.Value)

	if err != nil {
		return nil
	}

	cache := cache.FromContext(c)

	_, err = getSessionByHash(as.db, cache, jwePayload.Hash)

	if err != nil {
		return nil
	}

	now := time.Now().UTC()

	newExpires := now.Add(as.cnf.JWT.SessionTime)

	session := &schemas.Session{Name: jwePayload.Name,
		UserName: jwePayload.UserName,
		Hash:     jwePayload.Hash,
		Expires:  newExpires.Format(time.RFC3339)}

	jwePayload.IssuedAt = jwt.NewNumericDate(now)

	jwePayload.Expiry = jwt.NewNumericDate(newExpires)

	jweToken, err := auth.Encode(as.cnf.JWT.Secret, jwePayload)

	if err != nil {
		return nil
	}
	setSessionCookie(c, jweToken, int(as.cnf.JWT.SessionTime.Seconds()))
	return session
}

func (as *AuthService) Logout(c *gin.Context) (*schemas.Message, *types.AppError) {
	val, _ := c.Get("jwtUser")
	jwtUser := val.(*types.JWTClaims)
	client, _ := tgc.AuthClient(c, &as.cnf.TG, jwtUser.TgSession)

	tgc.RunWithAuth(c, client, "", func(ctx context.Context) error {
		_, err := client.API().AuthLogOut(c)
		return err
	})
	setSessionCookie(c, "", -1)
	as.db.Where("session = ?", jwtUser.TgSession).Delete(&models.Session{})
	cache := cache.FromContext(c)
	cache.Delete(fmt.Sprintf("sessions:%s", jwtUser.Hash))
	return &schemas.Message{Message: "logout success"}, nil
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
	tgClient, _ := tgc.NoAuthClient(c, &as.cnf.TG, dispatcher, sessionStorage)

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
					if !checkUserIsAllowed(as.cnf.JWT.AllowedUsers, user.Username) {
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
					if !checkUserIsAllowed(as.cnf.JWT.AllowedUsers, user.Username) {
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
					if !checkUserIsAllowed(as.cnf.JWT.AllowedUsers, user.Username) {
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

func checkUserIsAllowed(allowedUsers []string, userName string) bool {
	found := false
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

func setSessionCookie(c *gin.Context, value string, maxAge int) {
	c.SetSameSite(2)
	c.SetCookie("user-session", value, maxAge, "/", "", false, true)
}
