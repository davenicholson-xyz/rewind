/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/davenicholson-xyz/rewind/app"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string
var showVersionFlag bool
var appVersion string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "rewind",
	Short: "A brief description of your application",
	Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		if showVersionFlag {
			if appVersion == "" {
				fmt.Println("rewind version unknown")
			} else {
				fmt.Printf("%s\n", appVersion)
			}
			return
		}
		cmd.Help()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.Flags().BoolVarP(&showVersionFlag, "version", "v", false, "Show version")
}

// SetVersion sets the application version
func SetVersion(version string) {
	appVersion = version
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		viper.AddConfigPath(filepath.Join(home, ".config", "rewind"))
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}

// sendIPCMessage sends a message to the rewind daemon via Unix socket with timeout
func sendIPCMessage(action, path string) error {
	// Set timeout for the entire operation (5 seconds)
	timeout := 5 * time.Second

	// Connect to the Unix socket with timeout
	conn, err := net.DialTimeout("unix", "/tmp/rewind.sock", timeout)
	if err != nil {
		return fmt.Errorf("failed to connect to rewind daemon: %w", err)
	}
	defer conn.Close()

	// Set deadline for the entire connection
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return fmt.Errorf("failed to set connection deadline: %w", err)
	}

	// Create the message
	msg := Message{
		Action: action,
		Path:   path,
	}

	// Marshal the message to JSON
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Send the message
	if _, err := conn.Write(msgBytes); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	// Read the response
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Parse the response
	var response Response
	if err := json.Unmarshal(buffer[:n], &response); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Check if the operation was successful
	if !response.Success {
		return fmt.Errorf("daemon returned error: %s", response.Message)
	}

	app.Logger.WithField("response", response.Message).Info("IPC message sent successfully")
	return nil
}

// sendIPCMessageWithResponse sends a message to the rewind daemon and returns the response message
func sendIPCMessageWithResponse(action, path string) (string, error) {
	// Set timeout for the entire operation (5 seconds)
	timeout := 5 * time.Second

	// Connect to the Unix socket with timeout
	conn, err := net.DialTimeout("unix", "/tmp/rewind.sock", timeout)
	if err != nil {
		return "", fmt.Errorf("failed to connect to rewind daemon: %w", err)
	}
	defer conn.Close()

	// Set deadline for the entire connection
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return "", fmt.Errorf("failed to set connection deadline: %w", err)
	}

	// Create the message
	msg := Message{
		Action: action,
		Path:   path,
	}

	// Marshal the message to JSON
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal message: %w", err)
	}

	// Send the message
	if _, err := conn.Write(msgBytes); err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}

	// Read the response with larger buffer for status responses
	buffer := make([]byte, 4096)
	n, err := conn.Read(buffer)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Parse the response
	var response Response
	if err := json.Unmarshal(buffer[:n], &response); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Check if the operation was successful
	if !response.Success {
		return "", fmt.Errorf("daemon returned error: %s", response.Message)
	}

	return response.Message, nil
}
