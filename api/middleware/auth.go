package middleware

import (
	"net/http"
	"strings"

	"tempmail/model"
	"tempmail/store"

	"github.com/gin-gonic/gin"
)

const AccountKey = "account"

// Auth API Key 认证中间件
func Auth(s *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("Authorization")
		if apiKey == "" {
			apiKey = c.Query("api_key")
		}

		// 支持 Bearer token 格式
		apiKey = strings.TrimPrefix(apiKey, "Bearer ")
		apiKey = strings.TrimSpace(apiKey)

		if apiKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing api_key: use Authorization header or ?api_key= query param",
			})
			return
		}

		account, err := s.GetAccountByAPIKey(c.Request.Context(), apiKey)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid api_key",
			})
			return
		}

		c.Set(AccountKey, account)
		c.Next()
	}
}

// AdminOnly 管理员权限中间件
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		account := GetAccount(c)
		if account == nil || !account.IsAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "admin access required",
			})
			return
		}
		c.Next()
	}
}

func GetAccount(c *gin.Context) *model.Account {
	val, exists := c.Get(AccountKey)
	if !exists {
		return nil
	}
	a, ok := val.(*model.Account)
	if !ok {
		return nil
	}
	return a
}
