package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/canonical/pebble/client"
)

func main() {
	requester := NewMyRequester(os.Getenv("PEBBLE_SOCKET"))
	config := &client.Config{Requester: requester}
	pebble, err := client.New(config)
	if err != nil {
		log.Fatal(err)
	}
	myClient := NewMyClient(pebble)
	services, err := myClient.UpperServices()
	if err != nil {
		log.Fatal(err)
	}
	for _, info := range services {
		fmt.Printf("%s: startup %q, current %q\n", info.Name, info.Startup, info.Current)
	}
	fmt.Println("---")
	fmt.Printf("Server version: %s\n", myClient.ServerVersion)
}

type MyClient struct {
	ServerVersion string

	Maintenance      error
	WarningCount     int
	WarningTimestamp time.Time

	pebble *client.Client
}

func NewMyClient(pebble *client.Client) *MyClient {
	c := &MyClient{pebble: pebble}
	c.pebble.Requester().SetDecoder(c.decoder)
	return c
}

// NOTE: this is basically a copy of client.Client.decoder but with three
// lines of additional code to set the new ServerVersion field.
func (c *MyClient) decoder(ctx context.Context, res *http.Response, opts *client.RequestOptions, result interface{}) (*client.RequestResponse, error) {
	var serverResp response
	dec := json.NewDecoder(res.Body)
	if err := dec.Decode(&serverResp); err != nil {
		return nil, err
	}

	// New custom functionality: update the server version
	if serverResp.ServerVersion != "" {
		c.ServerVersion = serverResp.ServerVersion
	}

	// Update the maintenance error state
	if serverResp.Maintenance != nil {
		c.Maintenance = serverResp.Maintenance
	} else {
		c.Maintenance = nil
	}

	// Deal with error type response
	if serverResp.Type == "error" {
		var resultErr Error
		err := json.Unmarshal(serverResp.Result, &resultErr)
		if err != nil || resultErr.Message == "" {
			return nil, fmt.Errorf("server error: %q", serverResp.Status)
		}
		resultErr.StatusCode = serverResp.StatusCode
		return nil, &resultErr
	}

	// At this point only sync and async type requests may exist so lets
	// make sure this is the case.
	//
	// Tests depend on the order or checks, so lets keep the order unchanged
	// and deal with these before decode.
	if opts.Async == false {
		if serverResp.Type != "sync" {
			return nil, fmt.Errorf("expected sync response, got %q", serverResp.Type)
		}
	} else {
		if serverResp.Type != "async" {
			return nil, fmt.Errorf("expected async response for %q on %q, got %q", opts.Method, opts.Path, serverResp.Type)
		}
		if serverResp.StatusCode != http.StatusAccepted {
			return nil, fmt.Errorf("operation not accepted")
		}
		if serverResp.Change == "" {
			return nil, fmt.Errorf("async response without change reference")
		}
	}

	// Warnings are only included if not an error type response
	c.WarningCount = serverResp.WarningCount
	c.WarningTimestamp = serverResp.WarningTimestamp

	// Decode the supplied result type
	if result != nil {
		err := json.Unmarshal(serverResp.Result, &result)
		if err != nil {
			return nil, err
		}
		if dec.More() {
			return nil, fmt.Errorf("cannot parse json value")
		}
	}

	// Common response
	return &client.RequestResponse{
		StatusCode: serverResp.StatusCode,
		ChangeID:   serverResp.Change,
	}, nil

	return nil, nil
}

// Copied from client.go
type response struct {
	Result     json.RawMessage `json:"result"`
	Status     string          `json:"status"`
	StatusCode int             `json:"status-code"`
	Type       string          `json:"type"`
	Change     string          `json:"change"`

	WarningCount     int       `json:"warning-count"`
	WarningTimestamp time.Time `json:"warning-timestamp"`

	Maintenance *Error `json:"maintenance"`

	ServerVersion string `json:"server-version"`
}

// Copied from client.go
type Error struct {
	Kind    string      `json:"kind"`
	Value   interface{} `json:"value"`
	Message string      `json:"message"`

	StatusCode int
}

// Copied from client.go
func (e *Error) Error() string {
	return e.Message
}

func (c *MyClient) UpperServices() ([]UpperServiceInfo, error) {
	var infos []UpperServiceInfo
	_, err := c.pebble.Requester().Do(context.Background(), &client.RequestOptions{
		Method: "GET",
		Path:   "/v1/services",
	}, &infos)
	if err != nil {
		return nil, err
	}
	for _, info := range infos {
		info.Name = strings.ToUpper(info.Name)
		info.Startup = strings.ToUpper(info.Startup)
		info.Current = strings.ToUpper(info.Current)
	}
	return infos, nil
}

func (c *MyClient) GetWebsocket(url string) (*websocket.Conn, error) {
	dialer := websocket.Dialer{
		NetDial:          c.pebble.Requester().Transport().Dial,
		Proxy:            c.pebble.Requester().Transport().Proxy,
		TLSClientConfig:  c.pebble.Requester().Transport().TLSClientConfig,
		HandshakeTimeout: 5 * time.Second,
	}
	conn, _, err := dialer.Dial(url, nil)
	return conn, err
}

type UpperServiceInfo struct {
	Name    string `json:"name"`
	Startup string `json:"startup"`
	Current string `json:"current"`
}

type MyRequester struct {
	transport  *http.Transport
	httpClient *http.Client
	decoder    client.DecoderFunc
}

func NewMyRequester(socketPath string) *MyRequester {
	r := &MyRequester{}
	r.transport = &http.Transport{
		Dial: func(network, addr string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}
	r.httpClient = &http.Client{Transport: r.transport}
	return r
}

// NOTE: this is basically a copy of client.DefaultRequester.Do
func (r *MyRequester) Do(ctx context.Context, opts *client.RequestOptions, result interface{}) (*client.RequestResponse, error) {
	httpResp, err := r.doWithRetries(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Is the result expecting a caller-managed raw body?
	if opts.ReturnBody {
		return &client.RequestResponse{Body: httpResp.Body}, nil
	}

	// If we get here, this is a normal sync or async server request so
	// we have to close the body.
	defer httpResp.Body.Close()

	// Get the client decoder to extract what it needs before we proceed
	reqResp, err := r.decoder(ctx, httpResp, opts, result)
	if err != nil {
		return nil, err
	}

	return reqResp, nil
}

// NOTE: this is basically a copy of client.DefaultRequester.rawWithRetry
func (r *MyRequester) doWithRetries(ctx context.Context, opts *client.RequestOptions) (*http.Response, error) {
	retry := time.NewTicker(250 * time.Millisecond)
	defer retry.Stop()
	timeout := time.After(5 * time.Second)
	var rsp *http.Response
	var err error
	for {
		fullPath := "http://localhost" + opts.Path
		req, err := http.NewRequestWithContext(ctx, opts.Method, fullPath, opts.Body)
		if err != nil {
			return nil, err
		}
		for key, value := range opts.Headers {
			req.Header.Set(key, value)
		}

		rsp, err = r.httpClient.Do(req)
		if err == nil || opts.Method != "GET" {
			break
		}
		select {
		case <-retry.C:
			continue
		case <-timeout:
		}
		break
	}
	if err != nil {
		return nil, err
	}
	return rsp, nil
}

func (r *MyRequester) SetDecoder(decoder client.DecoderFunc) {
	r.decoder = decoder
}

func (r *MyRequester) Transport() *http.Transport {
	return r.transport
}
