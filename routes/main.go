package routes

import (
	"net/http"
	"strconv"

	"github.com/divyam234/teldrive/database"
	"github.com/gin-gonic/gin"
	"go.etcd.io/bbolt"
)

func AddRoutes(router *gin.Engine) {
	api := router.Group("/api")
	api.GET("/bbolt", Authmiddleware, func(c *gin.Context) {
		err := database.BoltDB.View(func(tx *bbolt.Tx) error {
			c.Writer.Header().Set("Content-Type", "application/octet-stream")
			c.Writer.Header().Set("Content-Disposition", `attachment; filename="teldrive.db"`)
			c.Writer.Header().Set("Content-Length", strconv.Itoa(int(tx.Size())))
			_, err := tx.WriteTo(c.Writer)
			return err
		})
		if err != nil {
			http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
		}

	})
	addAuthRoutes(api)
	addFileRoutes(api)
	addUploadRoutes(api)
	addUserRoutes(api)
}
