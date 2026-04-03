# Go Arrivals

Simple cli program to handle Odoo New Employees Arrivals.

## Build

Simply run: `go build`.

## Usage

```
$ ./arrivals -h
Usage of ./arrivals:
  -config string
        path to config dir (default: $GARRIVALS_CONFIG_PATH or ~/.config/odooArrivals)
  -force-download
        download even if local file is still fresh
  -format string
        output format: table or csv (default "table")
  -location string
        filter arrivals by location (default "Hong Kong")
  -memory
        process xlsx in memory without saving to disk
  -offline string
        process a local xlsx file instead of downloading
  -output string
        filepath to export downloaded file (default "arrivals.xlsx")
  -sheet string
        sheet name to parse (default "New colleagues 2026")
  -v    verbose output
```

The config path is resolved in this order:
1. `--config` flag
2. `GARRIVALS_CONFIG_PATH` environment variable
3. `os.UserConfigDir()` + `odooArrivals` (Linux: `~/.config/odooArrivals/`)

### Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Error |
| `2` | No upcoming arrivals found |

### Examples

```bash
# Default: download sheet, filter Hong Kong, print table
./arrivals

# Use a local file, output as CSV
./arrivals --offline arrivals.xlsx --format csv

# Force re-download, verbose
./arrivals --force-download -v

# Different location
./arrivals --location "Indonesia"
```

## First Time

1. Ask a SysAdmin or create a `credentials.json` via [the Google Cloud Console](https://console.cloud.google.com) which has access to `Google Drive` (readonly).
2. On the first run, it will show an url to setup the OAuth2, you will need to link your Google account and copy the `code` token from the final `URL`.
