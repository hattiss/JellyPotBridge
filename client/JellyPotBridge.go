package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/spf13/viper"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/term"
)

// JellyPotConfig holds the application configuration
type JellyPotConfig struct {
	ReportingInterval time.Duration  `mapstructure:"reporting-interval"`
	PotPlayerPath     string         `mapstructure:"pot-player-path"`
	Jellyfin          JellyfinConfig `mapstructure:"jellyfin"`
}

// JellyfinConfig contains Jellyfin server configuration
type JellyfinConfig struct {
	ServerUrl string `mapstructure:"server-url"`
	Username  string `mapstructure:"username"`
	Password  string `mapstructure:"password"`
	DeviceId  string `mapstructure:"device-id"`
}

// loadConfig reads and parses the configuration file
func loadConfig() (*JellyPotConfig, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}
	viper.AddConfigPath(filepath.Dir(exePath))
	viper.SetConfigName("config.yaml")
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config JellyPotConfig
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	return &config, nil
}

// Windows message constants for PotPlayer communication
const (
	WmUser              = 0x0400
	PotGetCurrentTime   = 0x5004 // Message to get current playback time
	PotGetPlayStatus    = 0x5006 // Message to get playback status
	TicksPerMillisecond = 10000  // Conversion factor for ticks
)

var (
	user32            = syscall.NewLazyDLL("user32.dll")
	procGetClassNameW = user32.NewProc("GetClassNameW")
)

// GetClassNameW retrieves the class name of a window
func GetClassNameW(hWnd syscall.Handle, className *uint16, nMaxCount int32) int32 {
	r1, _, _ := syscall.SyscallN(procGetClassNameW.Addr(),
		uintptr(hWnd),
		uintptr(unsafe.Pointer(className)),
		uintptr(nMaxCount))
	return int32(r1)
}

// PotPlayerClassNames contains possible window class names for PotPlayer
var PotPlayerClassNames = []string{
	"PotPlayer64",     // 64-bit default class name
	"PotPlayer",       // 32-bit default class name
	"PotPlayerMini64", // 64-bit mini mode class name
	"PotPlayerMini",   // 32-bit mini mode class name
}

// PotPlayerInfo holds playback information from PotPlayer
type PotPlayerInfo struct {
	HWnd         uintptr
	Status       int
	EventName    string
	Milliseconds uintptr
	Seconds      float64
	Ticks        int64
}

// findPotPlayerWindow locates the PotPlayer window using its class names
func findPotPlayerWindow() (uintptr, error) {
	// First try FindWindow with known class names
	for _, class := range PotPlayerClassNames {
		utf16Class, err := syscall.UTF16PtrFromString(class)
		if err != nil {
			return 0, fmt.Errorf("failed to convert class name: %w", err)
		}

		hWnd, _, err := user32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(utf16Class)), 0)

		if hWnd != 0 && errors.Is(err, syscall.Errno(0)) {
			return hWnd, nil
		}
	}

	// If not found, enumerate all windows
	var hWnd uintptr
	cb := syscall.NewCallback(func(h syscall.Handle, l uintptr) uintptr {
		var className [256]uint16
		GetClassNameW(h, &className[0], int32(len(className)))
		classNameStr := syscall.UTF16ToString(className[:])

		for _, c := range PotPlayerClassNames {
			if classNameStr == c {
				hWnd = uintptr(h)
				return 0 // Stop enumeration
			}
		}
		return 1 // Continue enumeration
	})

	_, _, _ = user32.NewProc("EnumWindows").Call(cb, 0)

	if hWnd != 0 {
		return hWnd, nil
	}

	return 0, fmt.Errorf("PotPlayer window not found")
}

// getPotPlayerInfo retrieves current playback information from PotPlayer
func getPotPlayerInfo() (*PotPlayerInfo, error) {
	hWnd, err := findPotPlayerWindow()
	if err != nil {
		return nil, fmt.Errorf("failed to find PotPlayer window: %w", err)
	}
	sendMessage := user32.NewProc("SendMessageW")
	if sendMessage.Find() != nil {
		return nil, fmt.Errorf("failed to get SendMessageW procedure")
	}

	// Get playback status
	status, _, err := sendMessage.Call(hWnd, uintptr(WmUser), uintptr(PotGetPlayStatus), 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get playback status: %w", err)
	}
	// Get current playback time in milliseconds
	milliseconds, _, err := sendMessage.Call(hWnd, uintptr(WmUser), uintptr(PotGetCurrentTime), 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get current playback time: %w", err)
	}
	seconds := float64(milliseconds) / 1000.0
	ticks := int64(milliseconds) * TicksPerMillisecond
	eventName := getEventName(int(status))

	return &PotPlayerInfo{
		HWnd:         hWnd,
		Status:       int(status),
		EventName:    eventName,
		Milliseconds: milliseconds,
		Seconds:      seconds,
		Ticks:        ticks,
	}, nil
}

// getEventName maps PotPlayer status codes to Jellyfin event names
func getEventName(status int) string {
	switch status {
	case 2:
		return "timeupdate"
	case 1:
		return "pause"
	case -1:
		return "stop"
	default:
		return "unknown"
	}
}

// PlaybackStatusEvent represents the playback status to send to Jellyfin
type PlaybackStatusEvent struct {
	PositionTicks          int64  `json:"PositionTicks"`
	PlaybackStartTimeTicks int64  `json:"PlaybackStartTimeTicks"`
	PlayMethod             string `json:"PlayMethod"`
	MediaSourceId          string `json:"MediaSourceId"`
	CanSeek                bool   `json:"CanSeek"`
	ItemId                 string `json:"ItemId"`
	EventName              string `json:"EventName"`
}

// MediaItem represents a media item from Jellyfin
type MediaItem struct {
	Id       string   `json:"Id"`
	Name     string   `json:"Name"`
	Type     string   `json:"Type"`
	UserData UserData `json:"UserData"`
}

// UserData represents a user data within a Jellyfin media item
type UserData struct {
	PlaybackPositionTicks int64  `json:"PlaybackPositionTicks"`
	ItemId                string `json:"ItemId"`
}

// JellyPotClient handles communication with the Jellyfin server
type JellyPotClient struct {
	serverUrl     string
	username      string
	password      string
	accessToken   string
	sessionId     string
	userId        string
	httpClient    *http.Client
	deviceName    string
	deviceId      string
	clientName    string
	clientVersion string
}

// NewJellyPotClient creates a new JellyPotClient instance
func NewJellyPotClient(serverUrl, username, password string, deviceId string) *JellyPotClient {
	return &JellyPotClient{
		serverUrl:     serverUrl,
		username:      username,
		password:      password,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		deviceName:    "PotPlayer",
		deviceId:      deviceId,
		clientName:    "JellyPot",
		clientVersion: gVersion,
	}
}

// Authenticate logs into the Jellyfin server and retrieves an access token
func (c *JellyPotClient) Authenticate() error {
	type authRequest struct {
		Username string `json:"Username"`
		Password string `json:"Pw"`
	}

	type sessionInfo struct {
		Id     string `json:"Id"`
		UserId string `json:"UserId"`
	}

	type authResponse struct {
		AccessToken string      `json:"AccessToken"`
		SessionInfo sessionInfo `json:"SessionInfo"`
	}

	body, err := json.Marshal(authRequest{
		Username: c.username,
		Password: c.password,
	})
	if err != nil {
		return fmt.Errorf("failed to create auth request: %w", err)
	}

	url := fmt.Sprintf("%s/Users/AuthenticateByName", c.serverUrl)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	c.setCommonHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send auth request: %w", err)
	}
	defer func(Body io.ReadCloser) { _ = Body.Close() }(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("authentication failed with status code: %d", resp.StatusCode)
	}

	var authResp authResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		c.accessToken = ""
		return fmt.Errorf("failed to parse auth response: %w", err)
	}

	c.accessToken = authResp.AccessToken
	c.sessionId = authResp.SessionInfo.UserId
	c.userId = authResp.SessionInfo.UserId
	return nil
}

// UpdatePlaybackStatus sends the current playback status to Jellyfin
func (c *JellyPotClient) UpdatePlaybackStatus(event PlaybackStatusEvent) error {
	if c.accessToken == "" {
		if err := c.Authenticate(); err != nil {
			return err
		}
	}

	reqBody, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to create playback status request: %w", err)
	}

	url := fmt.Sprintf("%s/Sessions/Playing/Progress", c.serverUrl)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	c.setCommonHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send playback status: %w", err)
	}
	defer func(Body io.ReadCloser) { _ = Body.Close() }(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		c.accessToken = "" // Force re-authentication on failure
		return fmt.Errorf("update playback status failed with code: %d", resp.StatusCode)
	}

	return nil
}

// GetItem retrieves details about a specific media item from Jellyfin
func (c *JellyPotClient) GetItem(itemId string) (*MediaItem, error) {
	if c.accessToken == "" {
		if err := c.Authenticate(); err != nil {
			return nil, err
		}
	}

	url := fmt.Sprintf("%s/Users/%s/Items/%s", c.serverUrl, c.userId, itemId)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	c.setCommonHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send item request: %w", err)
	}
	defer func(Body io.ReadCloser) { _ = Body.Close() }(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get item failed with status code: %d", resp.StatusCode)
	}

	var item MediaItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		c.accessToken = ""
		return nil, fmt.Errorf("failed to parse item response: %w", err)
	}

	return &item, nil
}

// setCommonHeaders adds standard headers required by Jellyfin API
func (c *JellyPotClient) setCommonHeaders(req *http.Request) {
	if c.accessToken == "" {
		req.Header.Set("Authorization", fmt.Sprintf(
			"MediaBrowser Client=\"%s\", Device=\"%s\", DeviceId=\"%s\", Version=\"%s\"",
			c.clientName, c.deviceName, c.deviceId, c.clientVersion,
		))
	} else {
		req.Header.Set("Authorization", fmt.Sprintf(
			"MediaBrowser Token=\"%s\", Client=\"%s\", Device=\"%s\", DeviceId=\"%s\", Version=\"%s\"",
			c.accessToken, c.clientName, c.deviceName, c.deviceId, c.clientVersion,
		))
	}
}

// RegisterProtocol registers a custom URL protocol to launch the current application
// protocol: The protocol name (e.g., "jellypot")
// description: Human-readable description of the protocol
func RegisterProtocol(protocol, description string) {
	exePath, err := os.Executable()
	if err != nil {
		fmt.Printf("Failed to get executable path: %s", err.Error())
		return
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		fmt.Printf("Failed to get absolute path: %s", err.Error())
		return
	}
	fmt.Printf("Registering protocol '%s' with handler: %s\n", protocol, exePath)
	key, _, err := registry.CreateKey(registry.CLASSES_ROOT, protocol, registry.ALL_ACCESS)
	if err != nil {
		fmt.Printf("Failed to create main protocol key: %s", err.Error())
		return
	}
	defer func(key registry.Key) { _ = key.Close() }(key)
	if err := key.SetStringValue("", "URL:"+description); err != nil {
		fmt.Printf("Failed to set protocol description: %s", err.Error())
		return
	}
	if err := key.SetStringValue("URL Protocol", ""); err != nil {
		fmt.Printf("Failed to set URL Protocol indicator: %s", err.Error())
		return
	}
	commandPath := fmt.Sprintf("%s\\shell\\open\\command", protocol)
	cmdKey, _, err := registry.CreateKey(registry.CLASSES_ROOT, commandPath, registry.ALL_ACCESS)
	if err != nil {
		fmt.Printf("Failed to create command key: %s", err.Error())
		return
	}
	defer func(cmdKey registry.Key) { _ = cmdKey.Close() }(cmdKey)
	launchCommand := fmt.Sprintf(`"%s" "%%1"`, exePath)
	if err := cmdKey.SetStringValue("", launchCommand); err != nil {
		fmt.Printf("Failed to set launch command: %s", err.Error())
		return
	}
	fmt.Printf("Successfully registered protocol: %s://\n", protocol)
}

// UnregisterProtocol removes a previously registered protocol from the system
// protocol: The protocol name to unregister
func UnregisterProtocol(protocol string) {
	fmt.Printf("Unregistering protocol: %s\n", protocol)
	if err := registry.DeleteKey(registry.CLASSES_ROOT, protocol); err != nil {
		fmt.Printf("Failed to delete protocol registry keys: %s", err.Error())
		return
	}
	fmt.Printf("Successfully unregistered protocol: %s://\n", protocol)
}

// getStartTimeTicks returns the current time in ticks for playback start time
func getStartTimeTicks() int64 {
	return time.Now().UnixNano() / 100
}

const gVersion = "1.0.0"

// printHelp
func printHelp() {
	fmt.Println("JellyPotBridge - Media playback tool connecting Jellyfin and PotPlayer")
	fmt.Println("Version: " + gVersion)
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  JellyPotBridge [command] [url]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  register          Register the jellypot:// protocol handler")
	fmt.Println("  unregister        Unregister the jellypot:// protocol handler")
	fmt.Println("  help              Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  JellyPotBridge register")
	fmt.Println("  JellyPotBridge jellypot://6b694a42d949478294df51e4ad9c5ef9")
}

// pressAnyKeyToContinue waits for the user to press any key before proceeding
func pressAnyKeyToContinue() {
	fmt.Print("Press any key to continue...")
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		panic(err)
	}
	defer func(fd int, oldState *term.State) { _ = term.Restore(fd, oldState) }(fd, oldState)
	b := make([]byte, 1)
	_, _ = os.Stdin.Read(b)
	fmt.Println()
}

// hideConsole hides the console window
func hideConsole() {
	if hWnd, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleWindow").Call(); hWnd != 0 {
		_, _, _ = syscall.NewLazyDLL("user32.dll").NewProc("ShowWindow").Call(hWnd, 0)
	}
}

const (
	PipeName = `\\.\pipe\JellyPotBridge_39AC4C3F`
)

// EnsureSingleInstance ensures only one instance runs, with new instances replacing old ones
func EnsureSingleInstance() bool {
	if exists, err := notifyExistingInstance(); exists {
		if err != nil {
			fmt.Printf("Warning: Failed to notify existing instance: %v\n", err)
		}
		time.Sleep(500 * time.Millisecond)
	}

	pipe, err := createPipeServer()
	if err != nil {
		fmt.Printf("Failed to initialize: %v\n", err)
		return false
	}
	go listenForNewInstances(pipe)
	return true
}

// createPipeServer establishes a new named pipe server
func createPipeServer() (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(PipeName)
	if err != nil {
		return 0, err
	}

	return windows.CreateNamedPipe(
		name,
		windows.PIPE_ACCESS_DUPLEX,
		windows.PIPE_TYPE_MESSAGE|windows.PIPE_READMODE_MESSAGE|windows.PIPE_WAIT,
		1, 4096, 4096, 500, nil,
	)
}

// listenForNewInstances waits for new instances and exits when requested
func listenForNewInstances(pipe windows.Handle) {
	defer func(handle windows.Handle) { _ = windows.CloseHandle(handle) }(pipe)
	buffer := make([]byte, 4096)
	var bytesRead uint32

	for {
		// Wait for connection
		if err := windows.ConnectNamedPipe(pipe, nil); err != nil && !errors.Is(err, windows.ERROR_PIPE_CONNECTED) {
			break
		}

		// Read message
		if err := windows.ReadFile(pipe, buffer, &bytesRead, nil); err == nil || errors.Is(err, windows.ERROR_MORE_DATA) {
			msg := windows.UTF16ToString((*[2048]uint16)(unsafe.Pointer(&buffer[0]))[:bytesRead/2])
			if msg == "EXIT" {
				_ = windows.DisconnectNamedPipe(pipe)
				os.Exit(0)
			}
		}

		_ = windows.DisconnectNamedPipe(pipe)
	}
}

// notifyExistingInstance checks for running instance and sends exit command
func notifyExistingInstance() (bool, error) {
	// Try to connect to existing pipe
	handle, err := windows.CreateFile(
		windows.StringToUTF16Ptr(PipeName),
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0,
	)

	if err != nil {
		return false, nil // No existing instance
	}
	defer func(handle windows.Handle) { _ = windows.CloseHandle(handle) }(handle)

	// Send exit command
	msg := windows.StringToUTF16("EXIT")
	var bytesWritten uint32
	err = windows.WriteFile(
		handle,
		(*[1 << 16]byte)(unsafe.Pointer(&msg[0]))[:len(msg)*2],
		&bytesWritten,
		nil,
	)

	return true, err
}

func main() {
	var itemId string
	if len(os.Args) > 1 {
		arg := os.Args[1]
		if arg == "help" {
			printHelp()
			return
		} else if arg == "register" {
			RegisterProtocol("jellypot", "jellypot protocol")
			return
		} else if arg == "unregister" {
			UnregisterProtocol("jellypot")
			return
		} else {
			if strings.HasPrefix(arg, "jellypot://") {
				itemId = strings.TrimSuffix(strings.TrimPrefix(arg, "jellypot://"), "/")
			} else {
				printHelp()
				pressAnyKeyToContinue()
				os.Exit(1)
			}
		}
	} else {
		printHelp()
		os.Exit(0)
	}

	// 1. Load configuration
	config, err := loadConfig()
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		pressAnyKeyToContinue()
		os.Exit(1)
	}

	// 2. Create JellyPot client and authenticate
	jellyPotClient := NewJellyPotClient(config.Jellyfin.ServerUrl, config.Jellyfin.Username, config.Jellyfin.Password,
		config.Jellyfin.DeviceId)

	if err := jellyPotClient.Authenticate(); err != nil {
		fmt.Printf("Jellyfin authentication failed: %v\n", err)
		pressAnyKeyToContinue()
		os.Exit(1)
	}
	fmt.Println("Jellyfin authentication successful")

	// 3. Retrieve media item information
	item, err := jellyPotClient.GetItem(itemId)
	if err != nil {
		fmt.Printf("Failed to get media item information: %v\n", err)
		pressAnyKeyToContinue()
		os.Exit(1)
	}
	fmt.Printf("Successfully retrieved media info: %s (Type: %s)\n", item.Name, item.Type)

	// 4. Launch PotPlayer
	if !EnsureSingleInstance() {
		fmt.Println("Failed to start - another instance is running")
		pressAnyKeyToContinue()
		os.Exit(1)
	}
	playbackUrl := fmt.Sprintf("%s/Items/%s/Download?api_key=%s", config.Jellyfin.ServerUrl, itemId,
		jellyPotClient.accessToken)
	fmt.Printf("Starting playback: %s\n", playbackUrl)

	cmd := exec.Command(config.PotPlayerPath,
		playbackUrl,
		"/title="+item.Name,
		"/seek="+strconv.FormatInt(item.UserData.PlaybackPositionTicks/TicksPerMillisecond/1000, 10),
		"/current",
	)
	if err := cmd.Start(); err != nil {
		fmt.Printf("Failed to start PotPlayer: %v\n", err)
		pressAnyKeyToContinue()
		os.Exit(1)
	}

	// 5. Monitor PotPlayer and send status updates at intervals
	time.Sleep(3 * time.Second) // Wait for PotPlayer to initialize
	ticker := time.NewTicker(config.ReportingInterval)
	defer ticker.Stop()

	potPlayerPid := cmd.Process.Pid
	fmt.Printf("PotPlayer started with PID: %d, reporting interval: %v\n",
		potPlayerPid, config.ReportingInterval)

	hideConsole()
	startTimeTicks := getStartTimeTicks()

	for {
		select {
		case <-ticker.C:
			info, err := getPotPlayerInfo()
			if err != nil {
				fmt.Println("PotPlayer has exited")
				os.Exit(0)
			}

			event := PlaybackStatusEvent{
				PositionTicks:          info.Ticks,
				PlaybackStartTimeTicks: startTimeTicks,
				PlayMethod:             "DirectPlay",
				MediaSourceId:          itemId,
				CanSeek:                true,
				ItemId:                 itemId,
				EventName:              info.EventName,
			}

			if err := jellyPotClient.UpdatePlaybackStatus(event); err != nil {
				fmt.Printf("Failed to send status update: %v\n", err)
			} else {
				fmt.Printf("Status updated: %s, Position: %d ticks\n",
					event.EventName, event.PositionTicks)
			}
		}
	}
}
