package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileData represents a downloaded file's data
type FileData struct {
	url  string
	body []byte
}

// DownloadManager defines an interface for downloading a file
type DownloadManager interface {
	Download(url string) ([]byte, error)
}

// HTTPDownloadManager implements the DownloadManager interface for HTTP downloads
type HTTPDownloadManager struct{}

// Download downloads a file using HTTP
func (m *HTTPDownloadManager) Download(url string) ([]byte, error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP GET failed with status %s", res.Status)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

// NewDownloadManager creates a new DownloadManager instance
func NewDownloadManager() DownloadManager {
	return &HTTPDownloadManager{}
}

// generateFilePath generates a filepath for a URL in a given destination directory
func generateFilePath(url *url.URL, destDir string) string {
	filename := filepath.Base(url.Path)
	if filename == "" {
		filename = "index.html"
	} else if !strings.Contains(filename, ".") {
		filename = filename + ".html"
	}
	return filepath.Join(destDir, filename)
}

// saveToFile saves data to a file at the given filepath
func saveToFile(filepath string, data []byte) error {
	return ioutil.WriteFile(filepath, data, os.ModePerm)
}

// crawl downloads a URL, saves it to disk, and recursively crawls any valid links
func crawl(url *url.URL, destDir string, downloaded map[string]bool, fileDataChan chan<- FileData, wg *sync.WaitGroup, dm DownloadManager) {
	defer wg.Done()

	// Check if the URL has already been downloaded
	if downloaded[url.String()] {
		return
	}

	// Mark the URL as downloaded
	downloaded[url.String()] = true

	// Download the URL
	body, err := dm.Download(url.String())
	if err != nil {
		fmt.Printf("Error downloading URL %s: %v\n", url.String(), err)
		return
	}

	// Send the file data over the channel for saving
	fileDataChan <- FileData{url.String(), body}

	// Find all links on the page
	links := extractLinks(url, body)

	// Recursively crawl each link
	for _, link := range links {
		// Skip links that have already been downloaded
		if downloaded[link.String()] {
			continue
		}

		// Skip links that are not valid children of the starting URL
		if !isChildLink(url, link) {
			continue
		}

		// Start crawling the link in a new goroutine
		wg.Add(1)
		go crawl(link, destDir, downloaded, fileDataChan, wg, dm)
	}
}

// extractLinks extracts all links from the given HTML document
// that are valid and returns them as a slice of URLs
func extractLinks(doc *html.Node, base *url.URL) []string {
	var urls []string

	if doc == nil {
		return urls
	}

	if doc.Type == html.ElementNode && doc.Data == "a" {
		for _, attr := range doc.Attr {
			if attr.Key == "href" {
				childURL, err := base.Parse(attr.Val)
				if err == nil && childURL.Host == base.Host && childURL.Scheme == base.Scheme {
					urls = append(urls, childURL.String())
				}
				break
			}
		}
	}

	for child := doc.FirstChild; child != nil; child = child.NextSibling {
		urls = append(urls, extractLinks(child, base)...)
	}

	return urls
}


