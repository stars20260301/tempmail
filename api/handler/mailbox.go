package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"tempmail/middleware"
	"tempmail/store"

	"github.com/gin-gonic/gin"
)

type MailboxHandler struct {
	store *store.Store
}

func NewMailboxHandler(s *store.Store) *MailboxHandler {
	return &MailboxHandler{store: s}
}

// POST /api/mailboxes - 创建临时邮箱（随机域名）
func (h *MailboxHandler) Create(c *gin.Context) {
	account := middleware.GetAccount(c)

	var req struct {
		Address string `json:"address"` // 可选，为空则随机生成
	}
	c.ShouldBindJSON(&req)

	// 生成邮箱地址
	address := strings.TrimSpace(req.Address)
	if address == "" {
		address = store.GenerateRandomAddress()
	}
	address = strings.ToLower(address)

	// 读取 TTL 设置
	ttlMinutes := 30
	if ttlStr, err := h.store.GetSetting(c.Request.Context(), "mailbox_ttl_minutes"); err == nil {
		if n, err := strconv.Atoi(ttlStr); err == nil && n > 0 {
			ttlMinutes = n
		}
	}

	// 从域名池随机选择一个活跃域名
	domain, err := h.store.GetRandomActiveDomain(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "no active domains available"})
		return
	}

	fullAddress := fmt.Sprintf("%s@%s", address, domain.Domain)

	mailbox, err := h.store.CreateMailbox(c.Request.Context(), account.ID, address, domain.ID, fullAddress, ttlMinutes)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			c.JSON(http.StatusConflict, gin.H{"error": "address already taken, try again"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"mailbox": mailbox})
}

// GET /api/mailboxes - 列出当前账号的邮箱
func (h *MailboxHandler) List(c *gin.Context) {
	account := middleware.GetAccount(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 { page = 1 }
	if size < 1 || size > 100 { size = 20 }

	mailboxes, total, err := h.store.ListMailboxes(c.Request.Context(), account.ID, page, size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  mailboxes,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

// DELETE /api/mailboxes/:id - 删除邮箱
func (h *MailboxHandler) Delete(c *gin.Context) {
	account := middleware.GetAccount(c)
	id, err := parseUUID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mailbox id"})
		return
	}

	if err := h.store.DeleteMailbox(c.Request.Context(), id, account.ID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "mailbox not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "mailbox deleted"})
}
