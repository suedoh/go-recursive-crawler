package main

import (
	"fmt"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/suedoh/go-recursive-crawler"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Printf("Usage: %s <url> <directory>\n", os.Args[0])
		os.Exit(1)
	}

	startURL := os.Args[1]
	destDir := os.Args[2]

	// Parse the start URL
	parsedURL, err := url.Parse(startURL)
	if err != nil {
		fmt.Printf("Error parsing URL: %v\n", err)
		os.Exit(1)
	}

	// Create the destination directory if it doesn't exist
	err = os.MkdirAll(destDir, os.ModePerm)
	if err != nil {
		fmt.Printf("Error creating directory: %v\n", err)
		os.Exit(1)
	}

	// Create a map to store files that have already been downloaded
	downloaded := make(map[string]bool)

	// Create a channel for file data and a wait group for handling concurrency
	fileDataChan := make(chan webcrawler.FileData)
	var wg sync.WaitGroup

	// Kick off the crawl
	wg.Add(1)
	go webcrawler.Crawl(parsedURL, destDir, downloaded, fileDataChan, &wg)

	// Wait for all the crawling to complete
	go func() {
		wg.Wait()
		close(fileDataChan)
	}()

	// Loop through the file data channel and save the files to disk
	for fileData := range fileDataChan {
		filepath := webcrawler.GenerateFilePath(fileData.URL, destDir)
		err := webcrawler.SaveToFile(filepath, fileData.Body)
		if err != nil {
			fmt.Printf("Error saving file: %v\n", err)
		}
	}

	fmt.Println("Crawling complete!")
}

