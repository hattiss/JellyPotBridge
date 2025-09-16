# JellyPotBridge
English / [中文](README_CN.md)

JellyPotBridge is a Go language-based tool suite that connects Jellyfin media server and PotPlayer, enabling seamless
integration and playback status synchronization between the two. The suite consists of two parts: a backend Go program
and a frontend Tampermonkey script.

## Features

### Go Backend Program

- Register a custom URL protocol (jellypot://) to support launching PotPlayer from web pages to play Jellyfin media
- Automatically retrieve media information from Jellyfin server
- Launch PotPlayer and resume playback from the last position
- Real-time monitoring of PotPlayer playback status (playing/paused/stopped)
- Regularly report playback progress to Jellyfin server
- Ensure only one instance of the application runs at a time

### Tampermonkey User Script

- Add PotPlayer playback button to Jellyfin web interface
- Support direct playback of single media content
- Automatically handle playback logic for series and seasons (play next episode for series, play first episode for
  seasons)
- Call the backend program via jellypot:// protocol

## System Requirements

- Windows operating system
- PotPlayer installed
- Jellyfin media server deployed
- Go 1.24 or higher (only required for development environment)

## Installation

### Go Backend Program

#### Compile and Install (For Developers)

1. Clone the project code

```bash
git clone <repository-url>
cd JellyPotBridge
```

2. Compile the project

```bash
go build -o bin/JellyPotBridge.exe client/JellyPotBridge.go
```

3. Copy the configuration file to the bin directory

```bash
copy client/config.yaml bin/
```

#### Direct Use (For Regular Users)

Directly obtain the compiled `JellyPotBridge.exe` and `config.yaml` files from the project's `bin` directory.

### Tampermonkey User Script

1. Install browser extension:
    -
    Chrome/Edge: [Tampermonkey](https://chrome.google.com/webstore/detail/tampermonkey/dhdgffkkebhmkfjojejmpbldmpobfkfo)
    - Firefox: [Tampermonkey](https://addons.mozilla.org/en-US/firefox/addon/tampermonkey/)

2. Open Tampermonkey dashboard and click "Add a new script"

3. Copy the entire content of `script/JellyPotBridge.js` file into the editor

4. Click "File" > "Save" to install the script

## Configuration

Before use, you need to configure the `config.yaml` file:

```yaml
reporting-interval: 10s
pot-player-path: "C:\\Program Files\\DAUM\\PotPlayer\\PotPlayerMini64.exe"
jellyfin:
  server-url: http://127.0.0.1:8096
  username: your_username
  password: your_password
  device-id: f7c8a374-365a-4545-94ed-94410338f495
```

- `reporting-interval`: Time interval for reporting playback status to Jellyfin server
- `pot-player-path`: Full path to the PotPlayer executable
- `jellyfin.server-url`: URL address of the Jellyfin server
- `jellyfin.username`: Jellyfin username
- `jellyfin.password`: Jellyfin password
- `jellyfin.device-id`: Device identifier, keep it unique

## Usage

### Go Backend Program

#### 1. Register URL Protocol

Before first use, you need to register the jellypot:// protocol:

```bash
JellyPotBridge.exe register
```

This will register the protocol handler in the Windows registry, allowing the browser to launch the application via
jellypot:// links.

#### 2. Play Media

After successful registration, you can launch media playback in the following ways:

- Click jellypot:// protocol links in the browser, format: `jellypot://<item-id>`
- Direct command line launch: `JellyPotBridge.exe jellypot://<item-id>`

Where `<item-id>` is the unique identifier of the media item in Jellyfin.

#### 3. Unregister URL Protocol

If you need to unregister the protocol, you can execute:

```bash
JellyPotBridge.exe unregister
```

#### 4. View Help Information

```bash
JellyPotBridge.exe help
```

### Tampermonkey User Script

After installing the script, visit the Jellyfin web interface, and a new PotPlayer play button will appear in the play
button area of the media detail page:

1. Open the Jellyfin web interface and log in
2. Navigate to the detail page of any media
3. A PotPlayer icon button will appear next to the play buttons (such as "Resume", "Play")
4. Click this button, and the script will automatically call the backend program and start PotPlayer to play the media

The script intelligently handles playback logic based on media type:

- For single episode content: directly play the current content
- For series: automatically get and play the next episode
- For seasons or collections: automatically get and play the first episode

## Notes

- Ensure PotPlayer is correctly installed in the path specified in the configuration file
- Ensure Jellyfin server is accessible and credentials are correct
- The application will monitor PotPlayer in the background while running, and will automatically exit when PotPlayer is
  closed
- Passwords in the configuration file are stored in plain text, please keep them secure