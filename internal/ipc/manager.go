package ipc

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/davenicholson-xyz/rewind/app"
	"github.com/davenicholson-xyz/rewind/internal/watcher"
	"github.com/davenicholson-xyz/rewind/network"
	"github.com/sirupsen/logrus"
)

type Message struct {
	Action string `json:"action"`
	Path   string `json:"path"`
}

type Response struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type Handler struct {
	ipc          *network.IPCClient
	WatchManager *watcher.WatchManager
}

func NewHandler(wm *watcher.WatchManager) (*Handler, error) {

	ipc, err := network.NewIPCClient(network.IPCConfig{AppName: "rewind"})
	if err != nil {
		return nil, err
	}

	return &Handler{ipc: ipc, WatchManager: wm}, nil
}

func (h *Handler) Start() {
	h.ipc.Start()
	defer h.ipc.Stop()

	var wg sync.WaitGroup
	wg.Add(1)

	stopChan := make(chan struct{})

	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopChan:
				return
			case msg, ok := <-h.ipc.Messages():
				if !ok {
					return
				}
				app.Logger.WithField("message", msg.Content).Debug("[IPC] Received message")
				h.processIPCMessage(msg)
			}
		}
	}()

	wg.Wait()
}
func (h *Handler) processIPCMessage(msg network.IPCMessage) error {
	var message Message
	if err := json.Unmarshal([]byte(msg.Content), &message); err != nil {
		app.Logger.WithError(err).Error("Failed to decode IPC message")
		response := Response{Success: false, Message: "Invalid message format"}
		json.NewEncoder(msg.Connection).Encode(response)
		return err
	}

	app.Logger.WithFields(logrus.Fields{
		"action": message.Action,
		"path":   message.Path,
	}).Info("Received IPC message")

	var response Response

	switch message.Action {
	case "add":
		err := h.WatchManager.AddWatch(message.Path)
		if err != nil {
			app.Logger.WithError(err).Error("Failed to add watch")
			response = Response{
				Success: false,
				Message: fmt.Sprintf("Failed to add watch for path %s: %v", message.Path, err),
			}
		} else {
			app.Logger.WithField("path", message.Path).Info("Successfully added watch")
			response = Response{
				Success: true,
				Message: fmt.Sprintf("Successfully added watch for path: %s", message.Path),
			}
		}
	case "remove":
		err := h.WatchManager.RemoveWatch(message.Path)
		if err != nil {
			app.Logger.WithError(err).Error("Failed to remove watch")
			response = Response{
				Success: false,
				Message: fmt.Sprintf("Failed to add watch for path %s: %v", message.Path, err),
			}
		} else {
			app.Logger.WithField("path", message.Path).Info("Successfully removed watch")
			response = Response{
				Success: true,
				Message: fmt.Sprintf("Successfully removed watch from path: %s", message.Path),
			}
		}
	case "status":
		status := h.WatchManager.GetStatus()
		statusJSON, err := json.Marshal(status)
		if err != nil {
			app.Logger.WithError(err).Error("Failed to marshal status")
			response = Response{
				Success: false,
				Message: fmt.Sprintf("Failed to get status: %v", err),
			}
		} else {
			response = Response{
				Success: true,
				Message: string(statusJSON),
			}
		}
	default:
		app.Logger.WithField("action", message.Action).Error("Unknown action")
		response = Response{
			Success: false,
			Message: fmt.Sprintf("Unknown action: %s", message.Action),
		}
	}

	if err := json.NewEncoder(msg.Connection).Encode(response); err != nil {
		app.Logger.WithError(err).Error("Failed to send IPC response")
		return err
	}

	app.Logger.WithFields(logrus.Fields{
		"success": response.Success,
		"message": response.Message,
	}).Debug("Sent IPC response")

	return nil
}
