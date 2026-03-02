package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/xuri/excelize/v2"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// Exit codes for cron/CI usage.
const (
	exitOK       = 0
	exitError    = 1
	exitNoResult = 2
)

const (
	spreadsheetId  = "1K4Gxx6xy0cU9JRb20uvGmY7N0IQIF8g8bNRoECxphmA"
	mimeType       = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	freshnessLimit = 1 * time.Hour
)

// verbose controls whether informational messages are printed.
var verbose bool

// logf prints a message to stderr only when verbose mode is enabled.
func logf(format string, args ...any) {
	if verbose {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}

// fatalf prints an error message to stderr and exits with code 1.
func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(exitError)
}

/* OAuth Token Handling */

func getClient(config *oauth2.Config, configDir string) *http.Client {
	tokFile := filepath.Join(configDir, "token.json")
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\ncode> ", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		fatalf("unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		fatalf("unable to retrieve token from web: %v", err)
	}
	return tok
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

func saveToken(path string, token *oauth2.Token) {
	logf("saving credential file to: %s", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		fatalf("unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

/* Config handling */

func resolveConfigPath(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if env := os.Getenv("GARRIVALS_CONFIG_PATH"); env != "" {
		return env
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		fatalf("unable to access config directory: %v", err)
	}
	return filepath.Join(dir, "odooArrivals")
}

func initDriveService(configPath string) (*drive.Service, error) {
	ctx := context.Background()

	configPath = resolveConfigPath(configPath)
	if err := os.MkdirAll(configPath, os.ModePerm); err != nil {
		fatalf("unable to create config directory: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(configPath, "credentials.json"))
	if err != nil {
		fatalf("unable to read client secret file: %v", err)
	}

	config, err := google.ConfigFromJSON(b, drive.DriveReadonlyScope)
	if err != nil {
		fatalf("unable to parse client secret file to config: %v", err)
	}
	client := getClient(config, configPath)

	return drive.NewService(ctx, option.WithHTTPClient(client))
}

/* Sheet file handling */

func downloadSheet(driveService *drive.Service) ([]byte, error) {
	resp, err := driveService.Files.Export(spreadsheetId, mimeType).Download()
	if err != nil {
		return nil, fmt.Errorf("failed to export file: %w", err)
	}
	defer resp.Body.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, resp.Body)
	return buf.Bytes(), err
}

func saveSheetToDisk(path string, data []byte) error {
	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to write to file: %w", err)
	}
	return nil
}

// isFileFresh returns true if the file exists and was modified within the freshness limit.
func isFileFresh(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < freshnessLimit
}

/* Arrivals processing */

type arrival struct {
	Name     string
	Date     string
	Function string
	Gram     string
}

// display new employees arrivals
func parseArrivals(xlsxData []byte, sheet, targetLocation string) ([]arrival, error) {
	f, err := excelize.OpenReader(bytes.NewReader(xlsxData))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, err
	}

	today := time.Now()
	var results []arrival
	for i, row := range rows {
		if i < 2 || len(row) < 8 {
			continue
		}

		name := row[0] + " " + row[1]
		date, function, where, gram := row[4], row[5], row[6], row[7]
		arrivalDate, _ := time.Parse("2006-01-02", date)

		if where == targetLocation && arrivalDate.After(today) {
			results = append(results, arrival{
				Name: name, Date: date, Function: function, Gram: gram,
			})
		}
	}
	return results, nil
}

func printTable(results []arrival) {
	fmt.Printf("%-25s | %-10s | %-50s | %s\n", "Name", "Date", "Function", "Gram")
	for _, r := range results {
		fmt.Printf("%-25s | %10s | %-50s | %s\n", r.Name, r.Date, r.Function, r.Gram)
	}
}

func printCSV(results []arrival) {
	w := csv.NewWriter(os.Stdout)
	w.Write([]string{"Name", "Date", "Function", "Gram"})
	for _, r := range results {
		w.Write([]string{r.Name, r.Date, r.Function, r.Gram})
	}
	w.Flush()
}

func main() {
	var (
		savePath       = flag.String("output", "arrivals.xlsx", "filepath to export downloaded file")
		offline        = flag.String("offline", "", "process a local xlsx file instead of downloading")
		sheet          = flag.String("sheet", "New colleagues 2026", "sheet name to parse")
		targetLocation = flag.String("location", "Hong Kong", "filter arrivals by location")
		inMemory       = flag.Bool("memory", false, "process xlsx in memory without saving to disk")
		configPath     = flag.String("config", "", "path to config dir (default: $GARRIVALS_CONFIG_PATH or ~/.config/odooArrivals)")
		forceDownload  = flag.Bool("force-download", false, "download even if local file is still fresh")
		format         = flag.String("format", "table", "output format: table or csv")
	)
	flag.BoolVar(&verbose, "v", false, "verbose output")
	flag.Parse()

	var xlsxData []byte

	// --- Load data ---
	if *offline != "" {
		logf("reading local file: %s", *offline)
		data, err := os.ReadFile(*offline)
		if err != nil {
			fatalf("failed to open offline xlsx file: %v", err)
		}
		xlsxData = data
	} else {
		// Check freshness before downloading.
		if !*forceDownload && !*inMemory && isFileFresh(*savePath) {
			logf("file %s is still fresh (< %v), skipping download (use --force-download to override)", *savePath, freshnessLimit)
			data, err := os.ReadFile(*savePath)
			if err != nil {
				fatalf("failed to read cached file: %v", err)
			}
			xlsxData = data
		} else {
			driveService, err := initDriveService(*configPath)
			if err != nil {
				fatalf("failed to create drive service: %v", err)
			}

			logf("downloading sheet from Google Drive...")
			xlsxData, err = downloadSheet(driveService)
			if err != nil {
				fatalf("failed to download file: %v", err)
			}
			logf("download complete (%d bytes)", len(xlsxData))

			if !*inMemory {
				if err := saveSheetToDisk(*savePath, xlsxData); err != nil {
					fatalf("failed to save xlsx file: %v", err)
				}
				logf("saved to %s", *savePath)
			}
		}
	}

	// --- Process & Output ---
	logf("parsing sheet %q, filtering location %q", *sheet, *targetLocation)
	results, err := parseArrivals(xlsxData, *sheet, *targetLocation)
	if err != nil {
		fatalf("failed to parse arrivals: %v", err)
	}

	if len(results) == 0 {
		logf("no upcoming arrivals found for location %q", *targetLocation)
		os.Exit(exitNoResult)
	}

	logf("found %d upcoming arrival(s)", len(results))

	switch *format {
	case "csv":
		printCSV(results)
	case "table":
		printTable(results)
	default:
		fatalf("unknown format %q (use 'table' or 'csv')", *format)
	}
}
