package webconsole

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

const defaultStaticDir = "web/dist"

func RegisterSPARoutes(r *gin.Engine, staticDir string) {
	staticDir = strings.TrimSpace(staticDir)
	if staticDir == "" {
		staticDir = defaultStaticDir
	}
	indexPath := filepath.Join(staticDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		return
	}

	r.GET("/console", func(c *gin.Context) {
		c.File(indexPath)
	})
	r.GET("/console/*path", func(c *gin.Context) {
		p := strings.TrimPrefix(c.Param("path"), "/")
		if p == "" {
			c.File(indexPath)
			return
		}
		full := filepath.Join(staticDir, p)
		if st, err := os.Stat(full); err == nil && !st.IsDir() {
			c.File(full)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.File(indexPath)
	})
}
