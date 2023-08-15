// Copyright (c) 2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/sessions"
)

// NOTE: This test does not test any vspd code.
//
// If the cookie store secret changes unexpectedly (common during development)
// the securecookie library returns an error with a hard-coded, non-exported
// string.
//
//	"securecookie: the value is not valid"
//
// TestCookieSecretError ensures the string returned by the lib does not change,
// which is important because vspd checks for the error using string comparison.
func TestCookieSecretError(t *testing.T) {

	// Create a cookie store, get a cookie from it.

	store := sessions.NewCookieStore([]byte("first secret"))

	req, err := http.NewRequest(http.MethodPost, "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	session, err := store.Get(req, "key")
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	err = store.Save(req, w, session)
	if err != nil {
		t.Fatal(err)
	}

	cookie := w.Result().Header["Set-Cookie"][0]

	// Create another cookie store using a different secret, send cookie from
	// first store to the new store, confirm error is correct.

	store2 := sessions.NewCookieStore([]byte("second secret"))

	req2, err := http.NewRequest(http.MethodPost, "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req2.Header.Add("Cookie", cookie)

	_, err = store2.Get(req2, "key")

	if !strings.Contains(err.Error(), invalidCookieErr) {
		t.Fatalf("securecookie library returned unexpected error, wanted %q, got %q",
			invalidCookieErr, err.Error())
	}
}
