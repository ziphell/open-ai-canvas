package handler

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// 交付包由后端直接托管 Vite 前端；Docker 部署仍由 Nginx 托管，不设置该目录时不改变现网行为。
const staticWebCacheRevalidate = "no-cache, must-revalidate"
const staticWebCacheImmutable = "public, max-age=31536000, immutable"

// Go 内置 MIME 表缺少这几种扩展名，浏览器会按错误类型执行或拒绝。
var staticWebContentTypes = map[string]string{
	".wasm": "application/wasm",
	".glb":  "model/gltf-binary",
	".gltf": "model/gltf+json",
}

// StaticWebHandler 托管构建后的前端目录，作为 NoRoute 的第一个处理器使用。
// 命中静态文件时直接响应并中止，/api 路径交给后续代理处理器。
func StaticWebHandler(distDir string) (gin.HandlerFunc, error) {
	root, err := filepath.Abs(distDir)
	if err != nil {
		return nil, fmt.Errorf("解析 CANVAS_WEB_DIST_DIR 失败：%w", err)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("CANVAS_WEB_DIST_DIR 不是可读目录：%s", distDir)
	}
	indexPath := filepath.Join(root, "index.html")
	if info, err := os.Stat(indexPath); err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("CANVAS_WEB_DIST_DIR 缺少 index.html：%s", distDir)
	}

	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Next()
			return
		}
		requestPath := c.Request.URL.Path
		if requestPath == "/api" || strings.HasPrefix(requestPath, "/api/") {
			c.Next()
			return
		}
		// 与 nginx 一致：反斜杠和发布器的内部文件不对外提供。
		if strings.Contains(requestPath, "\\") || strings.Contains(requestPath, "/.canvas-") {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		cleaned := path.Clean("/" + requestPath)
		if cleaned == "/" {
			serveStaticWebFile(c, indexPath, staticWebCacheRevalidate)
			return
		}
		if file, ok := staticWebFile(root, cleaned); ok {
			cacheControl := staticWebCacheRevalidate
			if strings.HasPrefix(cleaned, "/assets/") {
				cacheControl = staticWebCacheImmutable
			}
			serveStaticWebFile(c, file, cacheControl)
			return
		}
		// 带内容哈希的构建产物缺失时应返回 404，避免旧页面把 index.html 当脚本执行。
		if strings.HasPrefix(cleaned, "/assets/") {
			c.Header("Cache-Control", "no-store")
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		serveStaticWebFile(c, indexPath, staticWebCacheRevalidate)
	}, nil
}

func staticWebFile(root, cleaned string) (string, bool) {
	name := filepath.Join(root, filepath.FromSlash(cleaned))
	relative, err := filepath.Rel(root, name)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", false
	}
	info, err := os.Stat(name)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return name, true
}

func serveStaticWebFile(c *gin.Context, name, cacheControl string) {
	file, err := os.Open(name)
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", cacheControl)
	c.Header("Referrer-Policy", "no-referrer")
	if contentType, ok := staticWebContentTypes[strings.ToLower(filepath.Ext(name))]; ok {
		c.Header("Content-Type", contentType)
	}
	http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), file)
	c.Abort()
}
