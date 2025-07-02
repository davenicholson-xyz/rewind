package network

import (
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/davenicholson-xyz/rewind/app"
)

const (
	DefaultBufferSize    = 1024
	DefaultChannelBuffer = 100
)

type IPCMessage struct {
	Content    string
	Connection net.Conn
	Time       time.Time
	ResponseCh chan string
}

type IPCClient struct {
	listener    net.Listener
	path        string
	stopChan    chan struct{}
	messageChan chan IPCMessage
	bufferSize  int
	mu          sync.RWMutex
	running     bool
}

type IPCConfig struct {
	AppName       string
	Path          string
	BufferSize    int
	ChannelBuffer int
}

func NewIPCClient(config IPCConfig) (*IPCClient, error) {
	if config.BufferSize == 0 {
		config.BufferSize = DefaultBufferSize
	}
	if config.ChannelBuffer == 0 {
		config.ChannelBuffer = DefaultChannelBuffer
	}

	path := determinePath(config)
	listener, err := createListener(path)
	if err != nil {
		return nil, fmt.Errorf("failed to create IPC listener: %v", err)
	}

	return &IPCClient{
		listener:    listener,
		path:        path,
		stopChan:    make(chan struct{}),
		messageChan: make(chan IPCMessage, config.ChannelBuffer),
		bufferSize:  config.BufferSize,
		running:     false,
	}, nil
}

func (ipc *IPCClient) Start() error {
	ipc.mu.Lock()
	defer ipc.mu.Unlock()

	if ipc.running {
		return fmt.Errorf("IPC client already running")
	}

	ipc.running = true
	app.Logger.WithField("path", ipc.path).Debug("IPC listening")
	go ipc.listenLoop()

	return nil
}

func (ipc *IPCClient) listenLoop() {
	defer func() {
		ipc.mu.Lock()
		if ipc.listener != nil {
			ipc.listener.Close()
		}
		close(ipc.messageChan)
		ipc.running = false
		ipc.mu.Unlock()
	}()

	for {
		select {
		case <-ipc.stopChan:
			app.Logger.Debug("IPC client shutting down")
			return
		default:
			conn, err := ipc.listener.Accept()
			if err != nil {
				select {
				case <-ipc.stopChan:
					return
				default:
					app.Logger.WithError(err).Error("Accept error")
					continue
				}
			}
			go ipc.handleConnection(conn)
		}
	}
}

func (ipc *IPCClient) handleConnection(conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, ipc.bufferSize)
	n, err := conn.Read(buf)
	if err != nil {
		app.Logger.WithError(err).Error("Read error")
		return
	}

	responseCh := make(chan string, 1)

	message := IPCMessage{
		Content:    string(buf[:n]),
		Connection: conn,
		Time:       time.Now(),
		ResponseCh: responseCh,
	}

	select {
	case ipc.messageChan <- message:
		select {
		case response := <-responseCh:
			_, err := conn.Write([]byte(response))
			if err != nil {
				app.Logger.WithError(err).Error("Write error")
			}
		}
	default:
		app.Logger.Error("Message channel full, dropping connection")
		conn.Write([]byte("ERROR: Server busy"))
	}
}

func (ipc *IPCClient) Messages() <-chan IPCMessage {
	return ipc.messageChan
}

func (ipc *IPCClient) Stop() {
	ipc.mu.RLock()
	if !ipc.running {
		ipc.mu.RUnlock()
		return
	}
	ipc.mu.RUnlock()

	close(ipc.stopChan)
	os.Remove(ipc.path)
}

func (ipc *IPCClient) IsRunning() bool {
	ipc.mu.RLock()
	defer ipc.mu.RUnlock()
	return ipc.running
}

func (ipc *IPCClient) GetPath() string {
	return ipc.path
}

func sendAndReceive(conn net.Conn, message string) (string, error) {
	_, err := conn.Write([]byte(message))
	if err != nil {
		return "", fmt.Errorf("failed to send message: %v", err)
	}

	buf := make([]byte, DefaultBufferSize)
	n, err := conn.Read(buf)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	return string(buf[:n]), nil
}

func determinePath(config IPCConfig) string {
	if config.Path != "" {
		return config.Path
	}

	var defaultName string
	if config.AppName != "" {
		defaultName = config.AppName
	} else {
		defaultName = "app"
	}

	return "/tmp/" + defaultName + ".sock"
}

func createListener(path string) (net.Listener, error) {
	os.Remove(path)
	return net.Listen("unix", path)
}

func SendToIPC(path, message string) (string, error) {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return "", fmt.Errorf("failed to connect to IPC: %v", err)
	}
	defer conn.Close()

	return sendAndReceive(conn, message)
}
