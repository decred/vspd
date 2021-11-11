// Copyright (c) 2020-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg/v3"
	dcrdtypes "github.com/decred/dcrd/rpc/jsonrpc/types/v3"
	"github.com/decred/slog"
	"github.com/decred/vspd/database"
	"github.com/gin-gonic/gin"
)

const (
	sigCharset = "0123456789ABCDEFGHJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz+/="
	hexCharset = "1234567890abcdef"
	testDb     = "test.db"
	backupDb   = "test.db-backup"
)

var (
	seededRand           = rand.New(rand.NewSource(time.Now().UnixNano()))
	feeXPub              = "feexpub"
	maxVoteChangeRecords = 3
)

func randBytes(n int) []byte {
	slice := make([]byte, n)
	if _, err := seededRand.Read(slice); err != nil {
		panic(err)
	}
	return slice
}

func TestMain(m *testing.M) {
	// Set test logger to stdout.
	backend := slog.NewBackend(os.Stdout)
	log = backend.Logger("test")
	log.SetLevel(slog.LevelTrace)

	// Set up some global params.
	cfg.NetParams = chaincfg.MainNetParams()
	_, signPrivKey, _ = ed25519.GenerateKey(seededRand)

	// Create a database to use.
	// Ensure we are starting with a clean environment.
	os.Remove(testDb)
	os.Remove(backupDb)

	// Create a new blank database for all tests.
	var err error
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	err = database.CreateNew(testDb, feeXPub)
	if err != nil {
		panic(fmt.Errorf("error creating test database: %v", err))
	}
	db, err = database.Open(ctx, &wg, testDb, time.Hour, maxVoteChangeRecords)
	if err != nil {
		panic(fmt.Errorf("error opening test database: %v", err))
	}

	// Run tests.
	exitCode := m.Run()

	// Request database shutdown and wait for it to complete.
	cancel()
	wg.Wait()
	db.Close()
	os.Remove(testDb)
	os.Remove(backupDb)

	os.Exit(exitCode)
}

// randString randomly generates a string of the requested length, using only
// characters from the provided charset.
func randString(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

// Ensure that testNode satisfies Node.
var _ Node = (*testNode)(nil)

type testNode struct {
	canTicketVote        bool
	canTicketVoteErr     error
	getRawTransactionErr error
}

func (n *testNode) CanTicketVote(_ *dcrdtypes.TxRawResult, _ string, _ *chaincfg.Params) (bool, error) {
	return n.canTicketVote, n.canTicketVoteErr
}

func (n *testNode) GetRawTransaction(txHash string) (*dcrdtypes.TxRawResult, error) {
	return nil, n.getRawTransactionErr
}

func TestSetAltSig(t *testing.T) {
	const testAddr = "DsVoDXNQqyF3V83PJJ5zMdnB4pQuJHBAh15"
	tests := []struct {
		name                 string
		vspClosed            bool
		deformReq            int
		addr                 string
		getRawTransactionErr error
		canTicketNotVote     bool
		isExistingAltSig     bool
		canTicketVoteErr     error
		wantCode             int
	}{{
		name:     "ok",
		addr:     testAddr,
		wantCode: http.StatusOK,
	}, {
		name:      "vsp closed",
		vspClosed: true,
		wantCode:  http.StatusBadRequest,
	}, {
		name:      "bad request",
		deformReq: 1,
		wantCode:  http.StatusBadRequest,
	}, {
		name:     "bad addr",
		addr:     "xxx",
		wantCode: http.StatusBadRequest,
	}, {
		name:     "addr wrong type",
		addr:     "DkM3ZigNyiwHrsXRjkDQ8t8tW6uKGW9g61qEkG3bMqQPQWYEf5X3J",
		wantCode: http.StatusBadRequest,
	}, {
		name:                 "error getting raw tx from dcrd client",
		addr:                 testAddr,
		getRawTransactionErr: errors.New("get raw transaction error"),
		wantCode:             http.StatusInternalServerError,
	}, {
		name:             "error getting can vote from dcrd client",
		addr:             testAddr,
		canTicketVoteErr: errors.New("can ticket vote error"),
		wantCode:         http.StatusInternalServerError,
	}, {
		name:             "ticket can't vote",
		addr:             testAddr,
		canTicketNotVote: true,
		wantCode:         http.StatusBadRequest,
	}, {
		name:             "hist at max",
		addr:             testAddr,
		isExistingAltSig: true,
		wantCode:         http.StatusBadRequest,
	}}

	for _, test := range tests {
		ticketHash := randString(64, hexCharset)
		req := &setAltSigRequest{
			Timestamp:     time.Now().Unix(),
			TicketHash:    ticketHash,
			TicketHex:     randString(504, hexCharset),
			ParentHex:     randString(504, hexCharset),
			AltSigAddress: test.addr,
		}
		reqSig := randString(504, hexCharset)
		b, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		if test.isExistingAltSig {
			data := &database.AltSigData{
				AltSigAddr: test.addr,
				Req:        b,
				ReqSig:     reqSig,
				Res:        randBytes(1000),
				ResSig:     randString(96, sigCharset),
			}
			if err := db.InsertAltSig(ticketHash, data); err != nil {
				t.Fatalf("%q: unable to insert ticket: %v", test.name, err)
			}
		}

		cfg.VspClosed = test.vspClosed

		tNode := &testNode{
			canTicketVote:        !test.canTicketNotVote,
			canTicketVoteErr:     test.canTicketVoteErr,
			getRawTransactionErr: test.getRawTransactionErr,
		}
		w := httptest.NewRecorder()
		c, r := gin.CreateTestContext(w)

		handle := func(c *gin.Context) {
			c.Set("DcrdClient", tNode)
			c.Set("RequestBytes", b[test.deformReq:])
			setAltSig(c)
		}

		r.POST("/", handle)

		c.Request, err = http.NewRequest(http.MethodPost, "/", nil)
		if err != nil {
			t.Fatal(err)
		}

		c.Request.Header.Set("VSP-Client-Signature", reqSig)

		r.ServeHTTP(w, c.Request)

		if test.wantCode != w.Code {
			t.Errorf("%q: expected status %d, got %d", test.name, test.wantCode, w.Code)
		}

		altsig, err := db.AltSigData(ticketHash)
		if err != nil {
			t.Fatalf("%q: unable to get alt sig data: %v", test.name, err)
		}

		if test.wantCode != http.StatusOK && !test.isExistingAltSig {
			if altsig != nil {
				t.Fatalf("%q: expected no alt sig saved for errored state", test.name)
			}
			continue
		}

		if !bytes.Equal(b, altsig.Req) || altsig.ReqSig != reqSig {
			t.Fatalf("%q: expected alt sig data different than actual", test.name)
		}
	}
}
