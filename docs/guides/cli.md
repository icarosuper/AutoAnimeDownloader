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
- Anilist usernames (supports more than one, comma-separated)
- Save path
- Completed anime path (required; must be on the same volume as the save path)
- Check interval (minutes)
- Max episodes per anime
- Episode retry limit
- Delete watched episodes (true/false)
- Excluded lists

`config get` is also the authoritative key list for `config set`, which accepts **every** key it prints.

#### `config set <key> <value>`

Update a configuration value. Accepts **any** key `config get` prints, by its JSON name — there is
no hand-maintained subset, so a field added to the daemon is settable the day it ships.

```bash
autoanimedownloader config set anilist_usernames myusername
autoanimedownloader config set check_interval 15
autoanimedownloader config set delete_watched_episodes true
autoanimedownloader config set min_seeders 3
```

The key is matched ignoring case and underscores, so `max_search_pages`, `maxSearchPages` and
`MAX_SEARCH_PAGES` all reach the same field.

**How the value is read:**
- A key that already holds a **list** accepts the comma-separated form:
  `config set anilist_usernames me,otheraccount`
- Anything else is parsed as **JSON** when it parses — `15`, `true`, `2.5`, `["a","b"]`,
  `{"batch_window_seconds":30}` — and taken as a plain **string** when it does not, which is what
  makes a path with a comma in it survive: `config set completed_anime_path "/mnt/Anime, Series"`

**Examples:**
```bash
# Two Anilist accounts
autoanimedownloader config set anilist_usernames myusername,secondaccount

# Check interval, in minutes
autoanimedownloader config set check_interval 15

# A nested object goes in as JSON
autoanimedownloader config set notifications '{"webhooks":[],"batch_window_seconds":30}'
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
# Show last 1000 lines (default)
autoanimedownloader logs

# Show last 100 lines
autoanimedownloader logs --lines 100   # or: -n 100

# Filter by level (all/debug/info/warn/error)
autoanimedownloader logs --level error

# Filter by text
autoanimedownloader logs --search "nyaa"   # or: -q "nyaa"

# Combine filters
autoanimedownloader logs --level warn -q retry
```

**What it shows:**
- Recent log entries from the daemon
- Logs are read from the daemon's log file
- Useful for debugging and monitoring
- `NO_COLOR=1` disables ANSI colors in the output

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

# 3. Set completed anime path (Jellyfin library; the download/seeding dir is derived from this)
autoanimedownloader config set completed_anime_path /path/to/library

# 4. Start the verification loop
autoanimedownloader loop start

# 5. Check status
autoanimedownloader status

# 6. View monitored animes
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

- [WebUI Guide](webui.md) - Web interface guide
- [Development Guide](development.md) - For developers

