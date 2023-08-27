package services

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/divyam234/teldrive/types"
	"github.com/divyam234/teldrive/utils"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	"github.com/gin-gonic/gin"
)

type UserService struct {
}

func getChunk(ctx context.Context, tgClient *telegram.Client, location tg.InputFileLocationClass, offset int64, limit int) ([]byte, error) {

	req := &tg.UploadGetFileRequest{
		Offset:   offset,
		Limit:    int(limit),
		Location: location,
	}

	r, err := tgClient.API().UploadGetFile(ctx, req)

	if err != nil {
		return nil, err
	}

	switch result := r.(type) {
	case *tg.UploadFile:
		return result.Bytes, nil
	default:
		return nil, fmt.Errorf("unexpected type %T", r)
	}
}

func iterContent(ctx context.Context, tgClient *telegram.Client, location tg.InputFileLocationClass) (*bytes.Buffer, error) {
	offset := int64(0)
	limit := 1024 * 1024
	buff := &bytes.Buffer{}
	for {
		r, err := getChunk(ctx, tgClient, location, offset, limit)
		if err != nil {
			return buff, err
		}
		if len(r) == 0 {
			break
		}
		buff.Write(r)
		offset += int64(limit)
	}
	return buff, nil
}

func (us *UserService) GetProfilePhoto(c *gin.Context) {
	val, _ := c.Get("jwtUser")
	jwtUser := val.(*types.JWTClaims)
	userId, _ := strconv.ParseInt(jwtUser.Subject, 10, 64)
	client, _ := utils.GetAuthClient(c, jwtUser.TgSession, userId)

	err := client.Run(c, func(ctx context.Context) error {
		self, err := client.Self(c)
		if err != nil {
			return err
		}
		peer := self.AsInputPeer()
		if self.Photo == nil {
			return nil
		}
		photo, _ := self.Photo.AsNotEmpty()
		location := &tg.InputPeerPhotoFileLocation{Big: false, Peer: peer, PhotoID: photo.PhotoID}
		buff, err := iterContent(c, client, location)
		if err != nil {
			return err
		}
		content := buff.Bytes()
		c.Writer.Header().Set("Content-Type", "image/jpeg")
		c.Writer.Header().Set("Cache-Control", "public, max-age=86400")
		c.Writer.Header().Set("Content-Length", strconv.Itoa(len(content)))
		c.Writer.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", "profile.jpeg"))
		c.Writer.Write(content)
		return nil
	})
	if err != nil {
		http.Error(c.Writer, err.Error(), http.StatusBadRequest)
		return
	}
}
