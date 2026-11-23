package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// SiteDomainGuard enforces the configured canonical site domain for panel APIs
// and frontend pages while leaving AI gateway routes untouched.
type SiteDomainGuard struct {
	target atomic.Pointer[string]
}

func NewSiteDomainGuard() *SiteDomainGuard {
	return &SiteDomainGuard{}
}

func (g *SiteDomainGuard) Set(domain string) error {
	target, err := service.NormalizeSiteDomain(domain)
	if err != nil {
		return err
	}
	if target == "" {
		g.target.Store(nil)
		return nil
	}
	g.target.Store(&target)
	return nil
}

func (g *SiteDomainGuard) RequireHost() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isPanelAPIPath(c.Request.URL.Path) {
			c.Next()
			return
		}
		target := g.target.Load()
		if target == nil || sameSiteHost(c.Request.Host, *target) {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"code":    http.StatusForbidden,
			"message": "request host does not match the configured site domain",
		})
	}
}

func isPanelAPIPath(path string) bool {
	return path == "/api/v1" || strings.HasPrefix(path, "/api/v1/")
}

// RedirectPage redirects a frontend page or asset request to the canonical
// origin. It returns true when it wrote and aborted the response.
func (g *SiteDomainGuard) RedirectPage(c *gin.Context) bool {
	target := g.target.Load()
	if target == nil || sameSiteHost(c.Request.Host, *target) {
		return false
	}
	location := "//" + *target + c.Request.URL.RequestURI()
	c.Redirect(http.StatusTemporaryRedirect, location)
	c.Abort()
	return true
}

func (g *SiteDomainGuard) RedirectHost() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !g.RedirectPage(c) {
			c.Next()
		}
	}
}

func sameSiteHost(requestHost, targetHost string) bool {
	reqHost, reqPort := splitDomainHostPort(requestHost)
	tgtHost, tgtPort := splitDomainHostPort(targetHost)
	if reqHost != tgtHost {
		return false
	}
	if reqPort == tgtPort {
		return true
	}
	// Treat omitted port as equivalent to default HTTP/HTTPS ports when target has no explicit port.
	if tgtPort == "" && (reqPort == "80" || reqPort == "443") {
		return true
	}
	if reqPort == "" && (tgtPort == "80" || tgtPort == "443") {
		return true
	}
	return false
}

func splitDomainHostPort(raw string) (string, string) {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	host, port, err := net.SplitHostPort(trimmed)
	if err != nil {
		return strings.TrimSuffix(trimmed, "."), ""
	}
	return strings.TrimSuffix(host, "."), port
}
