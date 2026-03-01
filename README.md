# Go Arrivals

Simple cli program to handle Odoo New Employees Arrivals.

## Build

Simply run: `go build`.

## Usage

```
$ ./arrivals -h
Usage of ./arrivals:
  -config string
        path to config files (credentials.json, token.json)
  -location string
        filter new users from location (default "Hong Kong")
  -memory
        process the xlsx file in memory
  -offline string
        process the pointed xlsx file
  -output string
        filepath to export downloaded file (default "arrivals.xlsx")
  -sheet string
        sheet's name to parse (default "New colleagues 2026")
```

The default `config` path is the result from `os.UserConfigDir()` and `odooArrivals`.  
On Linux the default config path would be: `~/.config/odooArrivals/`.

## First Time

1. Ask a SysAdmin or create a `credentials.json` via [the Google Cloud Console](https://console.cloud.google.com) which has access to `Google Drive` (readonly).
2. On the first run, it will show an url to setup the OAuth2, you will need to link your Google account and copy the `code` token from the final `URL`.
