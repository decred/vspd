// Copyright (c) 2020-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (w *WebAPI) homepage(c *gin.Context) {
	cacheData := c.MustGet(cacheKey).(cacheData)

	c.HTML(http.StatusOK, "homepage.html", gin.H{
		"WebApiCache": cacheData,
		"WebApiCfg":   w.cfg,
	})
}
