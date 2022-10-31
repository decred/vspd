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

	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/vspd/types"
)

type Client struct {
	http.Client
	URL    string
	PubKey []byte
	sign   func(context.Context, string, stdaddr.Address) ([]byte, error)
}

type Signer interface {
	SignMessage(ctx context.Context, message string, address stdaddr.Address) ([]byte, error)
}

func NewClient(url string, pub []byte, s Signer) *Client {
	return &Client{URL: url, PubKey: pub, sign: s.SignMessage}
}

type BadRequestError struct {
	HTTPStatus int    `json:"-"`
	Code       int    `json:"code"`
	Message    string `json:"message"`
}

func (e *BadRequestError) Error() string { return e.Message }

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
	if method == http.MethodPost {
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
	reply, err := c.Do(httpReq)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, httpReq.URL.String(), err)
	}
	defer reply.Body.Close()

	status := reply.StatusCode
	is200 := status == 200
	is4xx := status >= 400 && status <= 499
	if !(is200 || is4xx) {
		return fmt.Errorf("%s %s: http %v %s", method, httpReq.URL.String(),
			status, http.StatusText(status))
	}

	respBody, err := io.ReadAll(reply.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	err = ValidateServerSignature(reply, respBody, c.PubKey)
	if err != nil {
		return fmt.Errorf("authenticate server response: %v", err)
	}

	var apiError *BadRequestError
	if is4xx {
		apiError = new(BadRequestError)
		resp = apiError
	}
	if resp != nil {
		err = json.Unmarshal(respBody, resp)
		if err != nil {
			return fmt.Errorf("unmarshal respose body: %w", err)
		}
	}
	if apiError != nil {
		apiError.HTTPStatus = status
		return apiError
	}
	return nil
}

func ValidateServerSignature(resp *http.Response, body []byte, pubKey []byte) error {
	sigBase64 := resp.Header.Get("VSP-Server-Signature")
	if sigBase64 == "" {
		return errors.New("no signature provided")
	}
	sig, err := base64.StdEncoding.DecodeString(sigBase64)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}
	if !ed25519.Verify(pubKey, body, sig) {
		return errors.New("invalid signature")
	}

	return nil
}
