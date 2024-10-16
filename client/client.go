// Copyright (c) 2022-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package client

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"

	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/slog"
	"github.com/decred/vspd/types/v3"
)

type Client struct {
	http.Client
	URL    string
	PubKey []byte
	// Sign is a function which must be provided to an instance of Client so
	// that it can sign request bodies using the PrivKey of the specified
	// address.
	Sign func(context.Context, string, stdaddr.Address) ([]byte, error)
	Log  slog.Logger
}

func (c *Client) VspInfo(ctx context.Context) (*types.VspInfoResponse, error) {
	var resp *types.VspInfoResponse
	err := c.get(ctx, "/api/v3/vspinfo", &resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) FeeAddress(ctx context.Context, req types.FeeAddressRequest,
	commitmentAddr stdaddr.Address) (*types.FeeAddressResponse, error) {

	requestBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var resp *types.FeeAddressResponse
	err = c.post(ctx, "/api/v3/feeaddress", commitmentAddr, &resp, json.RawMessage(requestBody))
	if err != nil {
		return nil, err
	}

	// verify initial request matches server
	if !bytes.Equal(requestBody, resp.Request) {
		return nil, fmt.Errorf("server response contains differing request")
	}

	return resp, nil
}

func (c *Client) PayFee(ctx context.Context, req types.PayFeeRequest,
	commitmentAddr stdaddr.Address) (*types.PayFeeResponse, error) {

	// TSpendPolicy and TreasuryPolicy are optional but must be an empty map
	// rather than nil.
	if req.TSpendPolicy == nil {
		req.TSpendPolicy = map[string]string{}
	}
	if req.TreasuryPolicy == nil {
		req.TreasuryPolicy = map[string]string{}
	}

	requestBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var resp *types.PayFeeResponse
	err = c.post(ctx, "/api/v3/payfee", commitmentAddr, &resp, json.RawMessage(requestBody))
	if err != nil {
		return nil, err
	}

	// verify initial request matches server
	if !bytes.Equal(requestBody, resp.Request) {
		return nil, fmt.Errorf("server response contains differing request")
	}

	return resp, nil
}

func (c *Client) TicketStatus(ctx context.Context, req types.TicketStatusRequest,
	commitmentAddr stdaddr.Address) (*types.TicketStatusResponse, error) {

	requestBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var resp *types.TicketStatusResponse
	err = c.post(ctx, "/api/v3/ticketstatus", commitmentAddr, &resp, json.RawMessage(requestBody))
	if err != nil {
		return nil, err
	}

	// verify initial request matches server
	if !bytes.Equal(requestBody, resp.Request) {
		return nil, fmt.Errorf("server response contains differing request")
	}

	return resp, nil
}

func (c *Client) SetVoteChoices(ctx context.Context, req types.SetVoteChoicesRequest,
	commitmentAddr stdaddr.Address) (*types.SetVoteChoicesResponse, error) {

	// TSpendPolicy and TreasuryPolicy are optional but must be an empty map
	// rather than nil.
	if req.TSpendPolicy == nil {
		req.TSpendPolicy = map[string]string{}
	}
	if req.TreasuryPolicy == nil {
		req.TreasuryPolicy = map[string]string{}
	}

	requestBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var resp *types.SetVoteChoicesResponse
	err = c.post(ctx, "/api/v3/setvotechoices", commitmentAddr, &resp, json.RawMessage(requestBody))
	if err != nil {
		return nil, err
	}

	// verify initial request matches server
	if !bytes.Equal(requestBody, resp.Request) {
		return nil, fmt.Errorf("server response contains differing request")
	}

	return resp, nil
}

func (c *Client) post(ctx context.Context, path string, addr stdaddr.Address, resp, req any) error {
	return c.do(ctx, http.MethodPost, path, addr, resp, req)
}

func (c *Client) get(ctx context.Context, path string, resp any) error {
	return c.do(ctx, http.MethodGet, path, nil, resp, nil)
}

func (c *Client) do(ctx context.Context, method, path string, addr stdaddr.Address, resp, req any) error {
	var reqBody io.Reader
	var sig []byte

	sendBody := method == http.MethodPost
	if sendBody {
		body, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		sig, err = c.Sign(ctx, string(body), addr)
		if err != nil {
			return fmt.Errorf("sign request: %w", err)
		}
		reqBody = bytes.NewReader(body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, c.URL+path, reqBody)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	if sig != nil {
		httpReq.Header.Set("VSP-Client-Signature", base64.StdEncoding.EncodeToString(sig))
	}

	if c.Log.Level() == slog.LevelTrace {
		dump, err := httputil.DumpRequestOut(httpReq, sendBody)
		if err == nil {
			c.Log.Tracef("Request to %s\n%s\n", c.URL, dump)
		} else {
			c.Log.Tracef("VSP request dump failed: %v", err)
		}
	}

	reply, err := c.Do(httpReq)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, httpReq.URL.String(), err)
	}
	defer reply.Body.Close()

	if c.Log.Level() == slog.LevelTrace {
		dump, err := httputil.DumpResponse(reply, true)
		if err == nil {
			c.Log.Tracef("Response from %s\n%s\n", c.URL, dump)
		} else {
			c.Log.Tracef("VSP response dump failed: %v", err)
		}
	}

	respBody, err := io.ReadAll(reply.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	status := reply.StatusCode

	if status != http.StatusOK {
		// If no response body, return an error with just the HTTP status.
		if len(respBody) == 0 {
			return fmt.Errorf("http status %d (%s) with no body",
				status, http.StatusText(status))
		}

		// Try to unmarshal the response body to a known vspd error.
		d := json.NewDecoder(bytes.NewReader(respBody))
		d.DisallowUnknownFields()

		var apiError types.ErrorResponse
		err = d.Decode(&apiError)
		if err == nil {
			return apiError
		}

		// If the response body could not be unmarshalled it might not have come
		// from vspd (eg. it could be from an nginx reverse proxy or some other
		// intermediary server). Return an error with the HTTP status and the
		// full body so that it may be investigated.
		return fmt.Errorf("http status %d (%s) with body %q",
			status, http.StatusText(status), respBody)
	}

	err = ValidateServerSignature(reply, respBody, c.PubKey)
	if err != nil {
		return fmt.Errorf("authenticate server response: %w", err)
	}

	err = json.Unmarshal(respBody, resp)
	if err != nil {
		return fmt.Errorf("unmarshal response body: %w", err)
	}

	return nil
}

func ValidateServerSignature(resp *http.Response, body []byte, serverPubkey []byte) error {
	sigBase64 := resp.Header.Get("VSP-Server-Signature")
	if sigBase64 == "" {
		return errors.New("no signature provided")
	}
	sig, err := base64.StdEncoding.DecodeString(sigBase64)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}
	if !ed25519.Verify(serverPubkey, body, sig) {
		return errors.New("invalid signature")
	}

	return nil
}
