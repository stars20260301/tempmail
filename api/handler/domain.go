package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"tempmail/middleware"
	"tempmail/store"

	"github.com/gin-gonic/gin"
)

type DomainHandler struct {
	store        *store.Store
	cfgIP        string // SMTP_SERVER_IP env
	cfgHostname  string // SMTP_HOSTNAME env
}

func NewDomainHandler(s *store.Store, smtpIP, smtpHostname string) *DomainHandler {
	return &DomainHandler{store: s, cfgIP: smtpIP, cfgHostname: smtpHostname}
}

// getServerIP 优先读 DB 设置，其次环境变量传入的值
func (h *DomainHandler) getServerIP(ctx context.Context) string {
	if ip, err := h.store.GetSetting(ctx, "smtp_server_ip"); err == nil && ip != "" {
		return ip
	}
	return h.cfgIP
}

// getServerHostname 返回 MX 记录应指向的邮件服务器 hostname
// 优先: DB 设置 smtp_hostname → 环境变量 → 空串（傻用 mail.提交域名 方式）
func (h *DomainHandler) getServerHostname(ctx context.Context) string {
	if hn, err := h.store.GetSetting(ctx, "smtp_hostname"); err == nil && hn != "" {
		return hn
	}
	return h.cfgHostname
}

// POST /api/admin/domains - 添加域名到池（管理员）
func (h *DomainHandler) Add(c *gin.Context) {
	var req struct {
		Domain string `json:"domain" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	domain, err := h.store.AddDomain(c.Request.Context(), req.Domain)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "domain already exists: " + err.Error()})
		return
	}

	// 获取服务器 IP 和 hostname（来自 DB 设置或环境变量）
	serverIP := h.getServerIP(c.Request.Context())
	hostname := h.getServerHostname(c.Request.Context())

	// 构建 DNS 指引
	var dnsRecords []gin.H
	if hostname != "" {
		dnsRecords = []gin.H{
			{"type": "MX",  "host": "@", "value": hostname, "priority": 10, "description": "邮件交换记录，指向本服务器"},
			{"type": "TXT", "host": "@", "value": fmt.Sprintf("v=spf1 ip4:%s ~all", serverIP), "description": "SPF 记录（可选）"},
		}
	} else {
		dnsRecords = []gin.H{
			{"type": "MX",  "host": "@", "value": fmt.Sprintf("mail.%s", req.Domain), "priority": 10, "description": "邮件交换记录"},
			{"type": "A",   "host": fmt.Sprintf("mail.%s", req.Domain), "value": serverIP, "description": "邮件服务器 A 记录"},
			{"type": "TXT", "host": "@", "value": fmt.Sprintf("v=spf1 ip4:%s ~all", serverIP), "description": "SPF 记录（可选）"},
		}
	}

	// 返回 DNS 配置指引
	c.JSON(http.StatusCreated, gin.H{
		"domain":      domain,
		"dns_records": dnsRecords,
		"instructions": fmt.Sprintf(
			"请在域名 %s 的 DNS 管理面板中添加以上记录。添加后约 5-30 分钟生效。",
			req.Domain),
	})
}

// GET /api/domains - 列出所有域名（共享域名池）
func (h *DomainHandler) List(c *gin.Context) {
	_ = middleware.GetAccount(c) // 确保已认证

	domains, err := h.store.ListDomains(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"domains": domains})
}

// DELETE /api/admin/domains/:id - 删除域名（管理员）
func (h *DomainHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid domain id"})
		return
	}

	if err := h.store.DeleteDomain(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "domain deleted"})
}

// PUT /api/admin/domains/:id/toggle - 启用/禁用域名（管理员）
func (h *DomainHandler) Toggle(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid domain id"})
		return
	}

	var req struct {
		Active bool `json:"active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.store.ToggleDomain(c.Request.Context(), id, req.Active); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "domain updated"})
}

// POST /api/admin/domains/mx-import - MX快捷接入（DNS检测并自动导入）
// body: {"domain":"example.com", "force":false}
func (h *DomainHandler) MXImport(c *gin.Context) {
	var req struct {
		Domain string `json:"domain" binding:"required"`
		Force  bool   `json:"force"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Domain = strings.ToLower(strings.TrimSpace(req.Domain))

	// 获取服务器 IP / hostname（来自 DB 设置或环境变量，不内置硬编码）
	serverIP := h.getServerIP(c.Request.Context())
	hostname := h.getServerHostname(c.Request.Context())

	// DNS MX 检测
	matched, mxHosts, mxStatus := store.CheckDomainMX(req.Domain, serverIP)

	if !matched && !req.Force {
		var dnsHint []gin.H
		if hostname != "" {
			dnsHint = []gin.H{
				{"type": "MX", "host": "@", "value": hostname, "priority": 10},
				{"type": "TXT", "host": "@", "value": fmt.Sprintf("v=spf1 ip4:%s ~all", serverIP)},
			}
		} else {
			mailSub := fmt.Sprintf("mail.%s", req.Domain)
			dnsHint = []gin.H{
				{"type": "MX", "host": "@", "value": mailSub, "priority": 10},
				{"type": "A", "host": mailSub, "value": serverIP},
				{"type": "TXT", "host": "@", "value": fmt.Sprintf("v=spf1 ip4:%s ~all", serverIP)},
			}
		}
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":     "MX检测未通过，如确定要导入请加 force:true",
			"mx_status": mxStatus,
			"mx_hosts":  mxHosts,
			"server_ip": serverIP,
			"domain":    req.Domain,
			"dns_hint":  dnsHint,
		})
		return
	}

	// 导入到域名池
	domain, err := h.store.AddDomain(c.Request.Context(), req.Domain)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			c.JSON(http.StatusConflict, gin.H{"error": "域名已存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"domain":     domain,
		"mx_status":  mxStatus,
		"mx_matched": matched,
		"message":    fmt.Sprintf("域名 %s 已导入域名池，Postfix 将在 60 秒内自动同步", req.Domain),
	})
}

// POST /api/admin/domains/mx-register - 提交域名等待自动MX验证（无需手动确认）
// body: {"domain":"example.com"}
func (h *DomainHandler) MXRegister(c *gin.Context) {
	var req struct {
		Domain string `json:"domain" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Domain = strings.ToLower(strings.TrimSpace(req.Domain))

	serverIP  := h.getServerIP(c.Request.Context())
	hostname  := h.getServerHostname(c.Request.Context())

	// MX 目标: 优先用服务器自己的 hostname，否则用用户域名的 mail 子域
	mxTarget := fmt.Sprintf("mail.%s", req.Domain)
	dnsRequired := []gin.H{
		{"type": "MX", "host": "@", "value": mxTarget, "priority": 10},
		{"type": "A",  "host": mxTarget, "value": serverIP},
		{"type": "TXT", "host": "@", "value": fmt.Sprintf("v=spf1 ip4:%s ~all", serverIP)},
	}
	if hostname != "" {
		mxTarget = hostname
		dnsRequired = []gin.H{
			{"type": "MX", "host": "@", "value": hostname, "priority": 10},
			{"type": "TXT", "host": "@", "value": fmt.Sprintf("v=spf1 ip4:%s ~all", serverIP)},
		}
	}

	// 先尝试立即检测；通过则直接激活
	matched, _, mxStatus := store.CheckDomainMX(req.Domain, serverIP)
	if matched {
		domain, err := h.store.AddDomain(c.Request.Context(), req.Domain)
		if err != nil {
			if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
				// 已存在则直接返回
				domains, _ := h.store.ListDomains(c.Request.Context())
				for _, d := range domains {
					if d.Domain == req.Domain {
						c.JSON(http.StatusOK, gin.H{
							"domain":    d,
							"status":    d.Status,
							"mx_status": mxStatus,
							"message":   "域名已存在且处于激活状态",
						})
						return
					}
				}
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{
			"domain":  domain,
			"status":  "active",
			"message": "MX验证通过，域名已立即加入域名池",
		})
		return
	}

	// MX未通过 → 加入 pending，等待后台自动轮询
	domain, err := h.store.AddDomainPending(c.Request.Context(), req.Domain)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"domain":       domain,
		"status":       domain.Status,
		"server_ip":    serverIP,
		"mx_status":    mxStatus,
		"message":      fmt.Sprintf("域名 %s 已进入待验证队列，后台每30秒自动检测MX记录，通过后自动加入域名池", req.Domain),
		"dns_required": dnsRequired,
	})
}

// POST /api/domains/submit — 任意已登录用户提交域名进行 MX 自动验证
// 与 MXRegister 逻辑相同，但不需要管理员权限
func (h *DomainHandler) Submit(c *gin.Context) {
	h.MXRegister(c) // 复用相同逻辑
}

// GET /api/admin/domains/:id/status - 查询域名状态（用于前端轮询）
func (h *DomainHandler) GetStatus(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid domain id"})
		return
	}

	domain, err := h.store.GetDomainByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "domain not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":            domain.ID,
		"domain":        domain.Domain,
		"status":        domain.Status,
		"is_active":     domain.IsActive,
		"mx_checked_at": domain.MxCheckedAt,
	})
}

