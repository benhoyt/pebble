package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/canonical/pebble/client"
)

func main() {
	config := &client.Config{
		Socket: os.Getenv("PEBBLE_SOCKET"),
	}
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

	pebble *client.Client
}

func NewMyClient(pebble *client.Client) *MyClient {
	c := &MyClient{pebble: pebble}
	c.pebble.SetDecodeHook(c.decodeHook)
	return c
}

func (c *MyClient) decodeHook(data []byte, method, path string, opts *client.RequestOptions) error {
	// Demonstrate use of opts: only do custom decode on v1 requests.
	if !strings.HasPrefix(path, "/v1/") {
		return nil
	}
	var frame struct {
		ServerVersion string `json:"server-version"`
	}
	err := json.Unmarshal(data, &frame)
	if err != nil {
		return err
	}
	if frame.ServerVersion != "" {
		c.ServerVersion = frame.ServerVersion
	}
	return nil
}

func (c *MyClient) UpperServices() ([]UpperServiceInfo, error) {
	var infos []UpperServiceInfo
	err := c.pebble.DoSync(context.Background(), "GET", "/v1/services", nil, &infos)
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
		NetDial:          c.pebble.Transport().Dial,
		Proxy:            c.pebble.Transport().Proxy,
		TLSClientConfig:  c.pebble.Transport().TLSClientConfig,
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
