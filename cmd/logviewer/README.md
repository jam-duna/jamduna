# Log Viewer

A web-based real-time log viewer for JAM node logs with filtering, split view, and hierarchical module filtering.

## Building

```bash
go build -o cmd/logviewer/logviewer ./cmd/logviewer
```

## Usage

```bash
# View logs in current directory
./cmd/logviewer/logviewer

# View logs in a specific directory
./cmd/logviewer/logviewer -dir /path/to/logs

# Use custom port (default: 8080)
./cmd/logviewer/logviewer -port 9000

# Use custom log file pattern (regex with node ID capture group)
./cmd/logviewer/logviewer -pattern 'node-(\d+)\.log'
```

### Command Line Options

| Option | Default | Description |
|--------|---------|-------------|
| `-port` | `8080` | HTTP server port |
| `-dir` | `.` | Directory containing log files |
| `-pattern` | `polkajam-(\d+)\.log` | Log file pattern (regex with node ID capture group) |

### Example

```bash
# Start viewer for polkajam logs
./cmd/logviewer/logviewer -dir /Users/michael/Github/jam/logs -port 8080
```

Terminal output:

```text
2025/12/17 18:00:00 Tailing /Users/michael/Github/jam/logs/polkajam-0.log (node 0)
2025/12/17 18:00:00 Tailing /Users/michael/Github/jam/logs/polkajam-1.log (node 1)
2025/12/17 18:00:00 Tailing /Users/michael/Github/jam/logs/polkajam-2.log (node 2)
2025/12/17 18:00:00 Tailing /Users/michael/Github/jam/logs/polkajam-3.log (node 3)
2025/12/17 18:00:00 Tailing /Users/michael/Github/jam/logs/polkajam-4.log (node 4)
2025/12/17 18:00:00 Log viewer running at http://localhost:8080
2025/12/17 18:00:00 Press Ctrl+C to stop
```

Then open http://localhost:8080 in your browser.

## Features

### Filtering

- **Text Filter**: Type in the filter box to search logs (regex supported)
- **Level Filter**: Filter by log level (ERROR, WARN+, INFO+, DEBUG+)
- **Node Toggles**: Click node buttons to show/hide specific nodes

### Module Filter (Sidebar)

The sidebar auto-detects modules from incoming logs and builds a hierarchical tree:

```text
jam_node
├── chain
│   ├── guarantors
│   └── ...
├── net
│   └── peer_manager
├── telemetry
│   ├── BlockTransferred
│   ├── BlockAuthored
│   └── ...
└── ...
```

- Click checkboxes to enable/disable module branches
- Message types (like `BlockTransferred`) appear under their parent module
- Enabling a message type automatically enables its parent modules
- Use "All" / "None" buttons for bulk enable/disable

### View Modes

- **Fused View**: All logs in a single scrollable panel (default)
- **Split View**: Each node gets its own panel, useful for comparing nodes side-by-side

Toggle with the "Split View" button.

### Connection Status

The status indicator shows:

- **Green (pulsing)**: Connected and receiving logs
- **Green (static)**: Connected, waiting for logs
- **Orange**: Connected but no logs received for 10+ seconds
- **Gray**: Disconnected (auto-reconnects)

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `/` | Focus filter input |
| `Escape` | Clear filter and unfocus |
| `Space` | Pause/Resume |

### Other Controls

- **Pause/Resume**: Stop processing new logs temporarily
- **Clear**: Remove all logs from display
- **Auto-scroll**: Toggle automatic scroll to bottom on new logs
