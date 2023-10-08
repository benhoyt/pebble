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
	myClient := NewMyClient(pebble, requester)
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

func NewMyClient(pebble *client.Client, requester *MyRequester) *MyClient {
	c := &MyClient{pebble: pebble}
	requester.updateMetadata = c.updateMetadata
	return c
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

	Maintenance *client.Error `json:"maintenance"`

	ServerVersion string `json:"server-version"`
}

func (c *MyClient) updateMetadata(serverVersion string, maintenance *client.Error, warningCount int, warningTimestamp time.Time) {
	if serverVersion != "" {
		c.ServerVersion = serverVersion
	}
	c.Maintenance = maintenance
	c.WarningCount = warningCount
	c.WarningTimestamp = warningTimestamp
}

func (c *MyClient) UpperServices() ([]UpperServiceInfo, error) {
	resp, err := c.pebble.Requester().Do(context.Background(), &client.RequestOptions{
		Type:   client.SyncRequest,
		Method: "GET",
		Path:   "/v1/services",
	})
	if err != nil {
		return nil, err
	}
	var infos []UpperServiceInfo
	err = resp.DecodeResult(&infos)
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
	transport      *http.Transport
	httpClient     *http.Client
	updateMetadata UpdateMetadataFunc
}

type UpdateMetadataFunc func(serverVersion string, maintenance *client.Error, warningCount int, warningTimestamp time.Time)

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
func (r *MyRequester) Do(ctx context.Context, opts *client.RequestOptions) (*client.RequestResponse, error) {
	httpResp, err := r.doWithRetries(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Is the result expecting a caller-managed raw body?
	if opts.Type == client.RawRequest {
		return &client.RequestResponse{Body: httpResp.Body}, nil
	}

	// If we get here, this is a normal sync or async server request so
	// we have to close the body.
	defer httpResp.Body.Close()

	var serverResp response
	dec := json.NewDecoder(httpResp.Body)
	if err := dec.Decode(&serverResp); err != nil {
		return nil, err
	}

	r.updateMetadata(serverResp.ServerVersion, serverResp.Maintenance, serverResp.WarningCount, serverResp.WarningTimestamp)

	// Deal with error type response
	if serverResp.Type == "error" {
		var resultErr client.Error
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
	if opts.Type == client.SyncRequest {
		if serverResp.Type != "sync" {
			return nil, fmt.Errorf("expected sync response, got %q", serverResp.Type)
		}
	} else if opts.Type == client.AsyncRequest {
		if serverResp.Type != "async" {
			return nil, fmt.Errorf("expected async response for %q on %q, got %q", opts.Method, opts.Path, serverResp.Type)
		}
		if serverResp.StatusCode != http.StatusAccepted {
			return nil, fmt.Errorf("operation not accepted")
		}
		if serverResp.Change == "" {
			return nil, fmt.Errorf("async response without change reference")
		}
	} else {
		panic("internal error: invalid request type")
	}

	// Common response
	return &client.RequestResponse{
		StatusCode: serverResp.StatusCode,
		ChangeID:   serverResp.Change,
		Result:     serverResp.Result,
	}, nil
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

func (r *MyRequester) Transport() *http.Transport {
	return r.transport
}
