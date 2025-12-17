# WebUI Guide

This guide covers the AutoAnimeDownloader web interface.

## Overview

The WebUI is a modern, embedded web interface that runs alongside the daemon. It provides a user-friendly way to monitor and configure AutoAnimeDownloader without using the command line.

## Accessing the WebUI

After starting the daemon, access the web interface at:

**http://localhost:8091**

The WebUI is embedded directly in the daemon binary, so no separate installation is required.

## Pages

### Status Page

The Status page (`/` or `/status`) is the main dashboard showing:

#### Daemon Status

- **Status Badge**: Visual indicator of current status
  - 🟢 **Running** - Daemon is active and checking for episodes
  - 🟡 **Checking** - Currently verifying for new episodes
  - 🔴 **Stopped** - Daemon is not running

- **Last Check**: Timestamp of the last verification
- **Error Indicator**: Shows if the last check encountered an error

#### Control Buttons

- **Start** - Start the daemon loop
- **Stop** - Stop the daemon loop
- **Check** - Trigger an immediate manual check

#### Monitored Animes

Displays a list of animes being monitored:
- Anime name (extracted from episode names)
- Number of episodes downloaded
- Latest episode information

**Real-time Updates**: The status page uses WebSocket to update automatically when the daemon status changes.

### Episodes Page

The Episodes page (`/episodes`) shows all downloaded episodes:

#### Episode List

Each episode displays:
- **Episode Name** - Full name of the episode
- **Episode ID** - Unique identifier
- **Hash** - Episode hash for tracking
- **Download Date** - When the episode was downloaded

**Features:**
- Sorted by download date (newest first)
- Search/filter capabilities (if implemented)
- Pagination for large lists

### Config Page

The Config page (`/config`) allows you to configure all daemon settings:

#### Configuration Fields

- **Anilist Username** (required)
  - Your Anilist username
  - Used to fetch your watching list

- **Save Path** (required)
  - Directory where episodes are saved
  - Must be an absolute path
  - Example: `/home/user/downloads` (Linux) or `C:\Downloads` (Windows)

- **Completed Anime Path** (required)
  - Directory where completed anime are moved
  - Must be an absolute path

- **Check Interval** (required)
  - How often to check for new episodes (in minutes)
  - Minimum: 1 minute
  - Recommended: 10-30 minutes

- **qBittorrent URL** (required)
  - URL of qBittorrent WebUI
  - Default: `http://localhost:8080`
  - Format: `http://host:port`

- **Max Episodes Per Anime** (required)
  - Maximum number of episodes to download per anime
  - Prevents downloading entire series at once
  - Recommended: 10-20

- **Episode Retry Limit** (required)
  - Maximum retry attempts for failed downloads
  - Default: 5

- **Delete Watched Episodes** (required)
  - Automatically delete episodes after watching
  - Options: `true` or `false`

- **Excluded List** (optional)
  - Comma-separated list of anime titles to exclude
  - Case-insensitive matching
  - Example: `One Piece, Naruto, Bleach`

#### Saving Configuration

1. Fill in all required fields
2. Click **Save** button
3. Wait for success confirmation
4. Configuration is saved immediately

**Validation:**
- Required fields must be filled
- Numeric fields must be valid numbers
- Paths must be absolute paths
- URLs must be valid HTTP/HTTPS URLs

### Logs Page

The Logs page (`/logs`) displays real-time daemon logs:

#### Features

- **Real-time Updates**: Logs update automatically via WebSocket
- **Log Levels**: Color-coded by severity
  - 🔵 DEBUG - Detailed information
  - 🟢 INFO - Normal operations
  - 🟡 WARN - Warnings
  - 🔴 ERROR - Errors

- **Filtering**: Filter by log level (if implemented)
- **Auto-scroll**: Automatically scrolls to newest logs
- **Copy**: Copy log entries to clipboard

## Navigation

The WebUI has a consistent navigation bar at the top:

- **Status** - Go to status/dashboard page
- **Episodes** - View downloaded episodes
- **Config** - Configure daemon settings
- **Logs** - View daemon logs

## Real-time Updates

The WebUI uses WebSocket connections for real-time updates:

- **Status changes** are reflected immediately
- **Log entries** appear in real-time
- **Automatic reconnection** if connection is lost

You'll see a connection indicator showing WebSocket status.

## Responsive Design

The WebUI is responsive and works on:
- Desktop browsers
- Tablets
- Mobile devices (with some limitations)

## Browser Compatibility

The WebUI works with modern browsers:
- Chrome/Edge (recommended)
- Firefox
- Safari
- Opera

**Note**: Some features may require JavaScript to be enabled.

## Keyboard Shortcuts

(If implemented)
- `Ctrl/Cmd + K` - Focus search
- `Esc` - Close modals/dialogs
- `Ctrl/Cmd + R` - Refresh data

## Tips

1. **Keep the Status page open**: Monitor daemon activity in real-time
2. **Check logs regularly**: Use the Logs page to debug issues
3. **Save configuration carefully**: Verify paths and URLs before saving
4. **Use manual check**: Test configuration with the "Check" button

## Troubleshooting

### WebUI not loading

**Symptoms**: Blank page or connection error

**Solutions:**
1. Verify daemon is running: Check system status
2. Check URL: Ensure you're using `http://localhost:8091`
3. Check browser console: Look for JavaScript errors
4. Verify API is accessible: Try `http://localhost:8091/api/v1/status`

### Real-time updates not working

**Symptoms**: Status doesn't update automatically

**Solutions:**
1. Check WebSocket connection indicator
2. Refresh the page
3. Check browser console for WebSocket errors
4. Verify firewall isn't blocking WebSocket connections

### Configuration not saving

**Symptoms**: Changes don't persist

**Solutions:**
1. Check for validation errors (red fields)
2. Verify all required fields are filled
3. Check browser console for errors
4. Verify daemon has write permissions to config directory

### Styling issues

**Symptoms**: Page looks broken or unstyled

**Solutions:**
1. Hard refresh: `Ctrl/Cmd + Shift + R`
2. Clear browser cache
3. Check browser console for CSS loading errors
4. Try a different browser

## See Also

- [Installation Guide](installation.md) - How to install and start the daemon
- [CLI Guide](cli-guide.md) - Command-line interface
- [Development Guide](development.md) - For developers

