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
	"github.com/decred/vspd/types"
)

type Client struct {
	http.Client
	URL    string
	PubKey []byte
	sign   func(context.Context, string, stdaddr.Address) ([]byte, error)
	log    slog.Logger
}

type Signer interface {
	SignMessage(ctx context.Context, message string, address stdaddr.Address) ([]byte, error)
}

func NewClient(url string, pub []byte, s Signer, log slog.Logger) *Client {
	return &Client{
		URL:    url,
		PubKey: pub,
		sign:   s.SignMessage,
		log:    log,
	}
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

func (c *Client) post(ctx context.Context, path string, addr stdaddr.Address, resp, req interface{}) error {
	return c.do(ctx, http.MethodPost, path, addr, resp, req)
}

func (c *Client) get(ctx context.Context, path string, resp interface{}) error {
	return c.do(ctx, http.MethodGet, path, nil, resp, nil)
}

func (c *Client) do(ctx context.Context, method, path string, addr stdaddr.Address, resp, req interface{}) error {
	var reqBody io.Reader
	var sig []byte

	sendBody := method == http.MethodPost
	if sendBody {
		body, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		sig, err = c.sign(ctx, string(body), addr)
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

	if c.log.Level() == slog.LevelTrace {
		dump, err := httputil.DumpRequestOut(httpReq, sendBody)
		if err == nil {
			c.log.Tracef("Request to %s\n%s\n", c.URL, dump)
		} else {
			c.log.Tracef("VSP request dump failed: %v", err)
		}
	}

	reply, err := c.Do(httpReq)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, httpReq.URL.String(), err)
	}
	defer reply.Body.Close()

	if c.log.Level() == slog.LevelTrace {
		dump, err := httputil.DumpResponse(reply, true)
		if err == nil {
			c.log.Tracef("Response from %s\n%s\n", c.URL, dump)
		} else {
			c.log.Tracef("VSP response dump failed: %v", err)
		}
	}

	respBody, err := io.ReadAll(reply.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	status := reply.StatusCode

	if status != http.StatusOK {
		// If no response body, just return status.
		if len(respBody) == 0 {
			return fmt.Errorf("http status %d (%s) with no body",
				status, http.StatusText(status))
		}

		// Try unmarshal response body to a known vspd error.
		var apiError types.ErrorResponse
		err = json.Unmarshal(respBody, &apiError)
		if err == nil {
			return apiError
		}

		return fmt.Errorf("http status %d (%s) with body %q",
			status, http.StatusText(status), respBody)
	}

	err = ValidateServerSignature(reply, respBody, c.PubKey)
	if err != nil {
		return fmt.Errorf("authenticate server response: %v", err)
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
