package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"github.com/rvolosatovs/turtlitto/pkg/api"
	"github.com/rvolosatovs/turtlitto/pkg/api/apitest"
	"github.com/rvolosatovs/turtlitto/pkg/trcapi"
	"github.com/rvolosatovs/turtlitto/pkg/trcapi/trctest"
	"github.com/rvolosatovs/turtlitto/pkg/webapi"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

var (
	unixSockPath = filepath.Join(os.TempDir(), "trc-sock-test")

	netLst net.Listener

	logger *zap.SugaredLogger
)

const (
	timeout      = time.Second
	messageCount = 3
)

func init() {
	http.DefaultClient.Timeout = timeout

	zapLogger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	logger = zapLogger.Sugar()

	flag.Parse()
	if *tcpSock == "" {
		logger.Debug("Creating Unix socket...")
		f, err := os.Open(unixSockPath)
		if !os.IsNotExist(err) {
			f.Close()

			logger.Infof("Removing %s...", unixSockPath)
			if err := os.Remove(unixSockPath); err != nil {
				logger.Fatalf("Failed to remove %s: %s", unixSockPath, err)
			}
		}

		logger.Debugf("Listening on UNIX socket on %s...", unixSockPath)
		netLst, err = net.Listen("unix", unixSockPath)
		if err != nil {
			logger.Fatalf("Failed to open UNIX socket on %s: %s", unixSockPath, err)
		}

	} else {
		logger.Debugf("Listening on TCP socket on %s...", *tcpSock)
		netLst, err = net.Listen("tcp", *tcpSock)
		if err != nil {
			logger.Fatalf("Failed to open TCP socket on %s: %s", *tcpSock, err)
		}
	}
}

//Test_items: init() in main.go
//Input_spec: -
//Output_spec: Pass or fail
//Envir_needs: -
func TestMain(m *testing.M) {
	if err := flag.Set("unixSocket", unixSockPath); err != nil {
		logger.Fatalf("Failed to set `socket` to %s: %s", unixSockPath, err)
	}

	if err := flag.Set("debug", "true"); err != nil {
		logger.Fatalf("Failed to set `debug`: %s", err)
	}

	logger.Info("Starting SRRS in goroutine...")
	go main()

	dial := func() (net.Conn, error) { return net.DialTimeout("tcp", defaultTCPAddress, time.Second) }
	retries := 20

	conn, err := dial()
	for i := 0; err != nil && i < retries; i++ {
		time.Sleep(100 * time.Millisecond)
		conn, err = dial()
	}
	if err != nil {
		logger.Fatalf("Failed to connect to SRRS at %s: %s", defaultTCPAddress, err)
	}

	if err := conn.Close(); err != nil {
		logger.Fatalf("Failed to close connection: %s", err)
	}

	logger = zap.S()

	ret := m.Run()

	if err := netLst.Close(); err != nil {
		logger.Infof("Failed to close Unix socket: %s", err)
	}

	logger.Infof("Exiting with return code: %d", ret)
	os.Exit(ret)
}

//Test_items: init() in main.go
//Input_spec: -
//Output_spec: Pass or fail
//Envir_needs: -
func TestAPI(t *testing.T) {
	a := assert.New(t)

	handshake := &api.Handshake{
		Version: trcapi.DefaultVersion,
		Token:   "test3",
	}

	var sessionKey string

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()

		req, err := http.NewRequest(http.MethodGet, "http://"+defaultTCPAddress+"/"+webapi.AuthEndpoint, nil)
		a.NoError(err)
		req.SetBasicAuth("", handshake.Token)

		logger.Debug("Sending authentication request...")
		resp, err := http.DefaultClient.Do(req)
		if !a.NoError(err) {
			logger.With("error", err).Error("Failed to authenticate")
			return
		}
		defer resp.Body.Close()

		b, err := ioutil.ReadAll(resp.Body)
		a.NoError(err)

		logger.With("key", string(b)).Debug("Got session key")
		sessionKey = string(b)
	}()

	msgCh := make(chan *api.Message)

	trcConnect := func() (*trctest.Conn, func(), error) {
		logger.Debug("Waiting for connection on Unix socket...")
		unixConn, err := netLst.Accept()
		if err != nil {
			return nil, nil, err
		}

		logger.Debug("Establishing mock TRC connection...")
		trc := trctest.Connect(
			unixConn, unixConn,
			trctest.WithHandler(api.MessageTypePing, func(msg *api.Message) (*api.Message, error) {
				return trctest.DefaultPingHandler(msg)
			}),
			trctest.WithHandler(api.MessageTypeHandshake, func(msg *api.Message) (*api.Message, error) {
				a.NotNil(msg.ParentID)
				a.NotEmpty(msg.ParentID)
				a.NotEmpty(msg.MessageID)
				a.NotEqual(msg.MessageID, msg.ParentID)
				a.NotEmpty(msg.Payload)
				return nil, nil
			}),
			trctest.WithHandler(api.MessageTypeState, func(msg *api.Message) (*api.Message, error) {
				msgCh <- msg
				return trctest.DefaultStateHandler(msg)
			}),
		)
		return trc, func() { unixConn.Close() }, trc.SendHandshake(handshake)
	}

	trc, closeFn, err := trcConnect()
	if closeFn != nil {
		defer closeFn()
	}
	if !a.NoError(err) {
		t.FailNow()
	}

	logger.Debug("Waiting for authentication...")
	wg.Wait()
	if t.Failed() {
		t.FailNow()
	}

	wsAddr := "ws://localhost" + defaultTCPAddress + "/" + webapi.StateEndpoint
	logger.With("addr", wsAddr).Debug("Opening a WebSocket...")
	wsConn, _, err := websocket.DefaultDialer.Dial(wsAddr, nil)
	if !a.NoError(err) {
		t.Fatal("Failed to open WebSocket")
	}
	defer wsConn.Close()

	err = wsConn.WriteJSON(sessionKey)
	if !a.NoError(err) {
		t.FailNow()
	}

	t.Run("TRC->SRRC/state", func(t *testing.T) {
		for i := 0; i < messageCount; i++ {
			t.Run(strconv.Itoa(i), func(t *testing.T) {
				a = assert.New(t)

				expected := apitest.RandomState()
				if err := expected.Validate(); err != nil {
					panic(errors.Wrap(err, "invalid state generated"))
				}

				logger.Debug("Sending random state from TRC...")
				err = trc.SendState(expected)
				a.NoError(err)

				var got api.State
				logger.Debug("Receiving random state on WebSocket...")
				err = wsConn.ReadJSON(&got)
				a.NoError(err)
				//a.Equal(expected, &got) // TODO: enable, once diffs are sent
			})
		}
	})

	t.Run("SRRC->TRC/turtles", func(t *testing.T) {
		for i := 0; i < messageCount; i++ {
			t.Run(strconv.Itoa(i), func(t *testing.T) {
				a = assert.New(t)

				expected := &api.State{
					Turtles: apitest.RandomTurtleStateMap(),
				}
				for len(expected.Turtles) == 0 {
					expected.Turtles = apitest.RandomTurtleStateMap()
				}
				if err := expected.Validate(); err != nil {
					panic(errors.Wrap(err, "invalid state generated"))
				}

				b, err := json.Marshal(expected.Turtles)
				a.NoError(err)

				req, err := http.NewRequest(http.MethodPost, "http://"+defaultTCPAddress+"/"+webapi.TurtleEndpoint, bytes.NewReader(b))
				a.NoError(err)
				req.SetBasicAuth("", sessionKey)

				errCh := make(chan error, 1)
				go func() {
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						errCh <- err
					}
					defer resp.Body.Close()

					a.Equal(resp.StatusCode, http.StatusOK)

					b, err := ioutil.ReadAll(resp.Body)
					if err != nil {
						errCh <- errors.Wrap(err, "failed to read response body")
					}

					if len(b) > 0 {
						errCh <- errors.Errorf("server returned error: %s", string(b))
					}
					errCh <- nil
				}()

				var msg *api.Message
				select {
				case <-time.After(timeout):
					t.Fatal("Timed out waiting for message to arrive at SRRS")
				case msg = <-msgCh:
				}

				a.Equal(msg.Type, api.MessageTypeState)
				a.Nil(msg.ParentID)
				a.NotEmpty(msg.MessageID)

				select {
				case <-time.After(timeout):
					t.Fatal("Timed out sending state to SRRS")
				case err = <-errCh:
					if !a.NoError(err) {
						return
					}
				}

				var got api.State
				err = json.Unmarshal(msg.Payload, &got)
				a.NoError(err)
				//a.Equal(expected, &got) // TODO: enable, once diffs are sent

				got = api.State{}
				err = wsConn.ReadJSON(&got)
				a.NoError(err)
				//a.Equal(expected, &got) // TODO: enable, once diffs are sent

				wg.Wait()
			})
		}
	})

	t.Run("SRRC->TRC/command", func(t *testing.T) {
		for i := 0; i < messageCount; i++ {
			t.Run(strconv.Itoa(i), func(t *testing.T) {
				a = assert.New(t)

				expected := &api.State{
					Command: apitest.RandomCommand(),
				}

				b, err := json.Marshal(expected.Command)
				a.NoError(err)

				req, err := http.NewRequest(http.MethodPost, "http://"+defaultTCPAddress+"/"+webapi.CommandEndpoint, bytes.NewReader(b))
				a.NoError(err)
				req.SetBasicAuth("", sessionKey)

				errCh := make(chan error, 1)
				go func() {
					logger.Debug("Sending command to SRRS...")
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						errCh <- err
					}
					defer resp.Body.Close()

					a.Equal(resp.StatusCode, http.StatusOK)

					b, err := ioutil.ReadAll(resp.Body)
					if err != nil {
						errCh <- errors.Wrap(err, "failed to read response body")
					}

					if len(b) > 0 {
						errCh <- errors.Errorf("server returned error: %s", string(b))
					}
					errCh <- nil
				}()

				var msg *api.Message
				select {
				case <-time.After(timeout):
					t.Fatal("Timed out waiting for message to arrive at SRRS")
				case msg = <-msgCh:
				}

				a.Equal(msg.Type, api.MessageTypeState)
				a.Nil(msg.ParentID)
				a.NotEmpty(msg.MessageID)

				select {
				case <-time.After(timeout):
					t.Fatal("Timed out sending command to SRRS")
				case err = <-errCh:
					if !a.NoError(err) {
						return
					}
				}

				var got api.State
				err = json.Unmarshal(msg.Payload, &got)
				a.NoError(err)
				//a.Equal(expected, &got) // TODO: enable, once diffs are sent

				got = api.State{}
				err = wsConn.ReadJSON(&got)
				a.NoError(err)
				//a.Equal(expected, &got) // TODO: enable, once diffs are sent

				wg.Wait()
			})
		}
	})
}
