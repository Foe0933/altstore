package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"howett.net/plist"
)

const ALTSTORE_SOURCE = "./source.json"
const HTTP_TIME_FMT = "Mon, 02 Jan 2006 15:04:05 MST"
const ICONS_ADDRESS = "https://foe0933.github.io/altstore/icons"

type App struct {
	Name                 string    `json:"name"`
	BundleIdentifier     string    `json:"bundleIdentifier"`
	DeveloperName        string    `json:"developerName"`
	Subtitle             string    `json:"subtitle"`
	LocalizedDescription string    `json:"localizedDescription"`
	IconURL              string    `json:"iconURL"`
	Category             string    `json:"category"`
	Versions             []Version `json:"versions"`
}

type Version struct {
	Version      string `json:"version"`
	BuildVersion string `json:"buildVersion"`
	Date         string `json:"date"`
	Size         int    `json:"size"`
	DownloadURL  string `json:"downloadURL"`
}

type Source struct {
	Name         string   `json:"name"`
	Subtitle     string   `json:"subtitle"`
	Description  string   `json:"description"`
	IconURL      string   `json:"iconURL"`
	HeaderURL    string   `json:"headerURL"`
	Website      string   `json:"website"`
	TintColor    string   `json:"tintColor"`
	Nsfw         bool     `json:"nsfw"`
	FeaturedApps []string `json:"featuredApps"`
	Apps         []App    `json:"apps"`
}

func main() {
	if len(os.Args) == 1 {
		fmt.Println(`Usage: alts <command>

Commands:
update       Updates all apps in source
add <url>    adds app from url to source`)
		return
	}
	source := readSource(ALTSTORE_SOURCE)
	switch os.Args[1] {
	case "update":
		checkForUpdates(&source)
	case "add":
		addApp(&source, os.Args[2])
	}
	saveSource(source, ALTSTORE_SOURCE)
}

func addApp(source *Source, url string) {
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal("Error downloading file: ", err)
	}
	date, _ := time.Parse(HTTP_TIME_FMT, resp.Header.Get("last-modified"))
	size, err := strconv.Atoi(resp.Header.Get("content-length"))
	if err != nil {
		log.Fatalf("Malformed content-length received from %s: %s", url, err)
	}
	b, _ := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("Error downloading ipa: ", err)
	}
	plistFile, err := extractPlist(b)
	info := infoFromPlist(plistFile)
	v := Version{
		Version:      info["CFBundleShortVersionString"].(string),
		BuildVersion: info["CFBundleVersion"].(string),
		Size:         size,
		DownloadURL:  url,
		Date:         date.Format(time.DateOnly),
	}
	app := App{
		Name:             info["CFBundleDisplayName"].(string),
		BundleIdentifier: info["CFBundleIdentifier"].(string),
		Versions:         []Version{v},
	}
	app.IconURL = ICONS_ADDRESS + app.BundleIdentifier + "-icon.png"
	extractIcon(b, app.BundleIdentifier + "-icon.png")
	source.Apps = append(source.Apps, app)
}

func checkForUpdates(source *Source) {
	for idx, _ := range source.Apps {
		app := &source.Apps[idx]
		latest := app.Versions[0]
		url := latest.DownloadURL
		resp, err := http.Get(url)
		if err != nil {
			log.Printf("Error downloading file for %s: %s", app.Name, err)
		}
		date, _ := time.Parse(HTTP_TIME_FMT, resp.Header.Get("last-modified"))
		size, err := strconv.Atoi(resp.Header.Get("content-length"))
		if err != nil {
			log.Printf("Malformed content-length received from %s: %s", url, err)
			continue
		}
		if size == latest.Size {
			continue
		}
		log.Println("New version for app: ", app.Name)
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Println("Error downloading ipa: ", err)
			continue
		}
		plistFile, err := extractPlist(b)
		info := infoFromPlist(plistFile)
		v := Version{
			Version:      info["CFBundleShortVersionString"].(string),
			BuildVersion: info["CFBundleVersion"].(string),
			Size:         size,
			DownloadURL:  url,
			Date:         date.Format(time.DateOnly),
		}
		app.Versions = []Version{v}
	}
}

func infoFromPlist(plistFile []byte) map[string]any {
	var info map[string]any
	plist.Unmarshal(plistFile, &info)
	return info
}

func extractIcon(ipa []byte, filepath string) {
	z, err := zip.NewReader(bytes.NewReader(ipa), int64(len(ipa)))
	if err != nil {
		log.Println("Decompressing IPA failed: ", err)
		return
	}
	for _, file := range z.File {
		if strings.Contains(file.Name, "AppIcon") && strings.HasSuffix(file.Name, ".png") {
			f, err := file.Open()
			if err != nil {
				log.Println("Failed to decompress AppIcon: ", file.Name, err)
				continue
			}
			b, _ := io.ReadAll(f)
			os.WriteFile(filepath, b, 0o666)
			return
		}
	}
	return
}
func extractPlist(ipa []byte) ([]byte, error) {
	z, err := zip.NewReader(bytes.NewReader(ipa), int64(len(ipa)))
	if err != nil {
		log.Println("Decompressing IPA failed: ", err)
		return []byte{}, err
	}
	for _, file := range z.File {
		if strings.HasSuffix(file.Name, ".app/Info.plist") {
			f, err := file.Open()
			if err != nil {
				log.Println("Failed to decompress Info.plist: ", file.Name, err)
				continue
			}
			return io.ReadAll(f)
		}
	}
	return []byte{}, errors.New("No Info.plist found")
}

func readSource(f string) Source {
	b, err := os.ReadFile(f)
	if err != nil {
		log.Fatal("Failed to read source file: ", err)
	}
	var s Source
	if err := json.Unmarshal(b, &s); err != nil {
		log.Fatal("Failed to parse JSON: ", err)
	}
	return s
}

func saveSource(s Source, f string) {
	b, err := json.MarshalIndent(s, "", "\t")
	if err != nil {
		log.Fatalf("Error saving JSON: %s\n%v", err, s)
	}
	if err = os.WriteFile(f, b, 0o644); err != nil {
		log.Fatalf("Error writing source file: %s\n%s", err, b)
	}
}