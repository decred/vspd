package webapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
)

// adminPage is the handler for "GET /admin". The admin template will be
// rendered if the current session is authenticated as an admin, otherwise the
// login template will be rendered.
func adminPage(c *gin.Context) {
	session := c.MustGet("session").(*sessions.Session)
	admin := session.Values["admin"]

	if admin == nil {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{})
		return
	}

	tickets, err := db.GetAllTickets()
	if err != nil {
		log.Errorf("GetAllTickets error: %v", err)
		c.String(http.StatusInternalServerError, "Error getting tickets from db")
		return
	}

	c.HTML(http.StatusOK, "admin.html", gin.H{
		"Tickets": tickets,
	})
}

// adminLogin is the handler for "POST /admin". If a valid password is provided,
// the current session will be authenticated as an admin.
func adminLogin(c *gin.Context) {
	password := c.PostForm("password")

	if password != cfg.AdminPass {
		log.Warnf("Failed login attempt from %s", c.ClientIP())
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"IncorrectPassword": true,
		})
		return
	}

	setAdminStatus(true, c)
}

// adminLogout is the handler for "POST /admin/logout". The current session will
// have its admin authentication removed.
func adminLogout(c *gin.Context) {
	setAdminStatus(nil, c)
}

// setAdminStatus stores the authentication status of the current session.
func setAdminStatus(admin interface{}, c *gin.Context) {
	session := c.MustGet("session").(*sessions.Session)
	session.Values["admin"] = admin
	err := session.Save(c.Request, c.Writer)
	if err != nil {
		log.Errorf("Error saving session: %v", err)
		c.String(http.StatusInternalServerError, "Error saving session")
		return
	}

	c.Redirect(http.StatusFound, "/admin")
	c.Abort()
}
