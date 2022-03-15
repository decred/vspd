// Copyright (c) 2020-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) homepage(c *gin.Context) {
	c.HTML(http.StatusOK, "homepage.html", gin.H{
		"WebApiCache": getCache(),
		"WebApiCfg":   s.cfg,
	})
}
