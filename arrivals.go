package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
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

const (
	spreadsheetId = "1K4Gxx6xy0cU9JRb20uvGmY7N0IQIF8g8bNRoECxphmA"
	mimeType      = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
)

/* OAUTH Token Handling */

// Retrieve a token, saves the token, then returns the generated client
func getClient(config *oauth2.Config, configDir string) *http.Client {
	tokFile := filepath.Join(configDir, "token.json")
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token
// You need to copy the `code` part of the final URL
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\ncode> ", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

// Retrieves a token from a local file
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

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

/* Config handling */

// setup the drive service using oauth token
func initDriveService(configPath string) (*drive.Service, error) {
	var (
		err error
		ctx = context.Background()
	)

	if configPath == "" {
		configPath, err = os.UserConfigDir()
		if err != nil {
			log.Fatalf("Unable to acces config directory: %v", err)
		}
		configPath = filepath.Join(configPath, "odooArrivals")
	}
	err = os.MkdirAll(configPath, os.ModePerm)
	if err != nil {
		log.Fatalf("Unable to create config directory: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(configPath, "credentials.json"))
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	config, err := google.ConfigFromJSON(b, drive.DriveReadonlyScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(config, configPath)

	return drive.NewService(ctx, option.WithHTTPClient(client))
}

/* Sheet file handling (download, parsing) */

func downloadSheet(driveService *drive.Service, outputPath string) ([]byte, error) {
	resp, err := driveService.Files.Export(spreadsheetId, mimeType).Download()
	if err != nil {
		return nil, fmt.Errorf("failed to export file: %w", err)
	}
	defer resp.Body.Close()

	var sheet bytes.Buffer
	_, err = io.Copy(&sheet, resp.Body)
	return sheet.Bytes(), err
}

func saveSheetToDisk(path string, sheet []byte) error {
	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, bytes.NewReader(sheet))
	if err != nil {
		return fmt.Errorf("failed to write to file: %w", err)
	}
	return nil
}

// display new employees arrivals
func processArrivals(xlsxData []byte, sheet, targetLocation string) error {
	f, err := excelize.OpenReader(bytes.NewReader(xlsxData))
	if err != nil {
		return err
	}
	defer f.Close()

	rows, err := f.GetRows(sheet)
	if err != nil {
		return err
	}

	today := time.Now()
	fmt.Printf("%-25s | %-10s | %-50s | %s\n", "Name", "Date", "Function", "Gram")
	for i, row := range rows {
		if i == 0 || i == 1 || len(row) < 8 {
			continue
		}

		name, date, function, where, gram := row[0]+" "+row[1], row[4], row[5], row[6], row[7]
		arrivalDate, _ := time.Parse("2006-01-02", date)

		if where == targetLocation && arrivalDate.After(today) {
			fmt.Printf("%-25s | %10s | %-50s | %s\n", name, date, function, gram)
		}
	}
	return nil
}

func main() {
	var (
		savePath       = flag.String("output", "arrivals.xlsx", "filepath to export downloaded file")
		offline        = flag.String("offline", "", "process the pointed xlsx file")
		sheet          = flag.String("sheet", "New colleagues 2026", "sheet's name to parse")
		targetLocation = flag.String("location", "Hong Kong", "filter new users from location")
		inMemory       = flag.Bool("memory", false, "process the xlsx file in memory")
		configPath     = flag.String("config", "", "path to config files (credentials.json, token.json)")
	)
	flag.Parse()

	if *offline != "" {
		xlsxData, err := os.ReadFile(*offline)
		if err != nil {
			log.Fatalf("failed to open offline xlsx file: %v", err)
		}
		processArrivals(xlsxData, *sheet, *targetLocation)
		return
	}

	driveService, err := initDriveService(*configPath)
	if err != nil {
		log.Fatalf("failed to create drive service: %v", err)
	}

	log.Print("Download sheet from Google Drive...")
	xlsxData, err := downloadSheet(driveService, *savePath)
	if err != nil {
		log.Fatalf("failed to download file: %v", err)
	}
	log.Printf("Successfully downloaded file: %s", spreadsheetId)

	if !*inMemory {
		err = saveSheetToDisk(*savePath, xlsxData)
		if err != nil {
			log.Fatalf("failed to save xlsx file to disk: %v", err)
		}
		log.Printf("Successfully saved xlsx file to disk: %s", spreadsheetId)
	}
	processArrivals(xlsxData, *sheet, *targetLocation)
}
