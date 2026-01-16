# DVR Stremio Addon

A lightweight Stremio addon that provides access to EasyProxy DVR recordings.

> **Note:** This addon must be able to reach your EasyProxy server. Run it on the same network as EasyProxy, or ensure EasyProxy is accessible from wherever you deploy this addon.

## Features

- Browse all completed DVR recordings
- Search recordings by name
- Play recordings directly in Stremio
- Delete recordings from Stremio

## Installation

### Using Pre-built Container

```bash
podman run -d --name dvr-stremio -p 7001:7001 \
  -e EASYPROXY_URL="https://your-easyproxy-url" \
  -e EASYPROXY_PASSWORD="your-password" \
  ghcr.io/whocdgnzo1w/dvr-stremio:latest
```

### Building from Source

```bash
podman build -t dvr-stremio .
podman run -d --name dvr-stremio -p 7001:7001 \
  -e EASYPROXY_URL="https://your-easyproxy-url" \
  -e EASYPROXY_PASSWORD="your-password" \
  dvr-stremio
```

## Configuration

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `EASYPROXY_URL` | URL of your EasyProxy instance | `http://localhost:8080` |
| `EASYPROXY_PASSWORD` | API password for EasyProxy | (empty) |
| `PORT` | Port to run the addon on | `7001` |

## Adding to Stremio

1. Start the addon container
2. Open Stremio
3. Go to Addons → Community Addons
4. Enter in the search bar: `http://localhost:7001/manifest.json`
5. Click Install

## API Endpoints

- `GET /manifest.json` - Addon manifest
- `GET /catalog/tv/dvr-recordings.json` - List all recordings
- `GET /catalog/tv/dvr-recordings/search=<query>.json` - Search recordings
- `GET /meta/tv/dvr:<id>.json` - Get recording metadata
- `GET /stream/tv/dvr:<id>.json` - Get stream URLs for a recording

## License

MIT
