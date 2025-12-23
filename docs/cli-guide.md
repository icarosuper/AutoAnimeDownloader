# CLI Guide

This guide covers all available commands in the AutoAnimeDownloader CLI.

## Overview

The CLI (`autoanimedownloader`) is a command-line interface for managing the AutoAnimeDownloader daemon. It communicates with the daemon via REST API.

## Global Flags

All commands support these global flags:

- `--endpoint <url>` - Daemon API endpoint (default: `http://localhost:8091`)
- `--json` - Output results in JSON format
- `--verbose` or `-v` - Enable verbose output

## Commands

### Process Management

#### `start`

Start the daemon process in the background.

```bash
autoanimedownloader start
```

**What it does:**
- Starts the daemon as a background process
- Creates a PID file for process tracking
- The daemon will be accessible at the configured port (default: 8091)

#### `stop`

Stop the running daemon process.

```bash
autoanimedownloader stop
```

**What it does:**
- Sends SIGTERM to the daemon process
- Removes the PID file
- Gracefully shuts down the daemon

### Daemon Loop Control

#### `loop start`

Start the daemon's verification loop.

```bash
autoanimedownloader loop start
```

**What it does:**
- Starts the automatic verification loop
- The daemon will periodically check for new episodes
- Status changes to "running"

#### `loop stop`

Stop the daemon's verification loop.

```bash
autoanimedownloader loop stop
```

**What it does:**
- Stops the automatic verification loop
- No more automatic checks will be performed
- Status changes to "stopped"

### Status and Information

#### `status`

Show the current daemon status.

```bash
autoanimedownloader status
```

**Output includes:**
- Current status (stopped/running/checking)
- Last check timestamp
- Whether the last check had an error

**Example output:**
```
Status: running
Last Check: 2024-01-15 10:30:45
Has Error: false
```

**JSON output:**
```bash
autoanimedownloader status --json
```

### Configuration

#### `config get`

Display the current configuration.

```bash
autoanimedownloader config get
```

**Shows all configuration values:**
- Anilist username
- Save path
- Completed anime path
- Check interval (minutes)
- qBittorrent URL
- Max episodes per anime
- Episode retry limit
- Delete watched episodes (true/false)
- Excluded list

#### `config set <key> <value>`

Update a configuration value.

```bash
autoanimedownloader config set anilist_username myusername
autoanimedownloader config set check_interval 15
autoanimedownloader config set delete_watched_episodes true
```

**Available keys:**
- `anilist_username` (string) - Your Anilist username
- `save_path` (string) - Path where episodes are saved
- `completed_anime_path` (string) - Path for completed anime
- `check_interval` (int) - Check interval in minutes
- `qbittorrent_url` (string) - qBittorrent WebUI URL
- `max_episodes_per_anime` (int) - Maximum episodes to download per anime
- `episode_retry_limit` (int) - Maximum retry attempts for episodes
- `delete_watched_episodes` (bool) - Delete episodes after watching (true/false)
- `excluded_list` (string) - Comma-separated list of excluded anime titles

**Examples:**
```bash
# Set Anilist username
autoanimedownloader config set anilist_username myusername

# Set check interval to 15 minutes
autoanimedownloader config set check_interval 15

# Set save path
autoanimedownloader config set save_path /path/to/downloads

# Enable deletion of watched episodes
autoanimedownloader config set delete_watched_episodes true
```

### Manual Operations

#### `check`

Trigger a manual verification immediately.

```bash
autoanimedownloader check
```

**What it does:**
- Forces an immediate check for new episodes
- Does not wait for the scheduled interval
- Returns immediately (check runs asynchronously)

### Data Viewing

#### `animes`

List all monitored animes with aggregated information.

```bash
autoanimedownloader animes
```

**Output includes:**
- Anime name (extracted from episode names)
- Number of episodes downloaded
- Latest episode ID
- Latest episode name

**Example output:**
```
Anime Name              Episodes  Latest Episode ID  Latest Episode
One Piece              150       12345              One Piece - Episode 1000
Naruto                 50        12346              Naruto - Episode 500
```

#### `episodes`

List all downloaded episodes.

```bash
autoanimedownloader episodes
```

**Output includes:**
- Episode ID
- Episode name
- Hash
- Download date

**Example output:**
```
Episode ID  Episode Name                    Hash                              Download Date
12345       One Piece - Episode 1000        abc123def456...                   2024-01-15 10:30:00
12346       Naruto - Episode 500            def456ghi789...                   2024-01-15 09:15:00
```

### Logs

#### `logs`

View daemon logs.

```bash
# Show last 50 lines (default)
autoanimedownloader logs

# Show last 100 lines
autoanimedownloader logs --lines 100

# Show last 20 lines
autoanimedownloader logs --lines 20
```

**What it shows:**
- Recent log entries from the daemon
- Logs are read from the daemon's log file
- Useful for debugging and monitoring

### Web UI

#### `open`

Open the web UI in your default browser.

```bash
autoanimedownloader open
```

**What it does:**
- Opens `http://localhost:8091` (or configured port) in your default browser
- Convenient shortcut to access the web interface

## Output Formats

### Table Format (Default)

By default, commands output data in a human-readable table format:

```bash
autoanimedownloader status
```

### JSON Format

Use the `--json` flag for machine-readable JSON output:

```bash
autoanimedownloader status --json
```

This is useful for scripting and automation.

## Error Handling

If the daemon is not running or unreachable, most commands will show an error:

```
Error: failed to connect to daemon: dial tcp 127.0.0.1:8091: connect: connection refused
```

**Solutions:**
1. Make sure the daemon is running: `autoanimedownloader start`
2. Check if the port is correct: `autoanimedownloader --endpoint http://localhost:8091 status`
3. Verify the daemon is accessible: check firewall settings

## Examples

### Complete Setup Workflow

```bash
# 1. Start the daemon
autoanimedownloader start

# 2. Configure Anilist username
autoanimedownloader config set anilist_username myusername

# 3. Configure qBittorrent URL
autoanimedownloader config set qbittorrent_url http://localhost:8080

# 4. Set save path
autoanimedownloader config set save_path /path/to/downloads

# 5. Start the verification loop
autoanimedownloader loop start

# 6. Check status
autoanimedownloader status

# 7. View monitored animes
autoanimedownloader animes
```

### Monitoring and Debugging

```bash
# Check current status
autoanimedownloader status

# View recent logs
autoanimedownloader logs --lines 100

# Force a manual check
autoanimedownloader check

# View downloaded episodes
autoanimedownloader episodes
```

### Configuration Management

```bash
# View current configuration
autoanimedownloader config get

# Update multiple settings
autoanimedownloader config set check_interval 30
autoanimedownloader config set max_episodes_per_anime 20
autoanimedownloader config set delete_watched_episodes false

# Verify changes
autoanimedownloader config get
```

## Tips

1. **Use JSON output for scripting**: Combine `--json` with tools like `jq` for advanced processing
2. **Check logs regularly**: Use `logs` command to monitor daemon activity
3. **Manual checks**: Use `check` command to test configuration without waiting for the interval
4. **Status monitoring**: Use `status` to verify the daemon is working correctly

## Troubleshooting

### Command not found

If `autoanimedownloader` is not found:
- Make sure it's installed and in your PATH
- On Linux: Check `~/.local/bin/` or use full path
- On Windows: Check installation directory

### Connection refused

If you get connection errors:
- Verify daemon is running: `autoanimedownloader start`
- Check the endpoint: Use `--endpoint` flag if daemon is on a different host/port
- Check firewall settings

### Permission errors

On Linux, you may need to ensure:
- The CLI binary has execute permissions: `chmod +x autoanimedownloader`
- You have permission to access the daemon's API

## See Also

- [Installation Guide](installation.md) - How to install the CLI
- [WebUI Guide](webui-guide.md) - Web interface guide
- [Development Guide](development.md) - For developers

