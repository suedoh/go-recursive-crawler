package main

import (
    "net/http"
    "os"
    "os/signal"
    "path"
    "path/filepath"
    "sync"
    "bytes"
    "fmt"
    "io"
    "net/url"
    "flag"
    "log"
    "strings"
    "io/ioutil"

    "golang.org/x/net/html"
)

type URLData struct {
    URL          *url.URL
    Filename     string
    Downloaded   bool
    RelativePath string
    Depth        int
}

type Spider struct {
    RootURL     string
    OutputDir   string
    Concurrency int
    MaxDepth    int
    MaxURLs     int
    domain      string
    urlsVisited int
    urls        chan URLData
}

var client = http.Client{}
var (
    mutex sync.Mutex
    wg    sync.WaitGroup
)

func main() {
    // Parse command line arguments
    flag.Parse()

    // Check if starting URL is valid
    _, err := url.ParseRequestURI(*startURL)
    if err != nil {
        log.Fatalf("Invalid starting URL: %s", err)
    }

    // Create destination directory if it doesn't exist
    if _, err := os.Stat(*destDir); os.IsNotExist(err) {
        err := os.MkdirAll(*destDir, os.ModePerm)
        if err != nil {
            log.Fatalf("Error creating destination directory: %s", err)
        }
    }

    // Create a channel to receive URLs to crawl
    toCrawl := make(chan string)

    // Create a channel to receive URLs to skip
    toSkip := make(chan string)

    // Create a channel to signal when all URLs have been crawled
    done := make(chan bool)

    // Create a crawl manager with the specified options
    c := CrawlManager{
        StartURL: *startURL,
        DestDir:  *destDir,
        MaxDepth: *maxDepth,
        MaxPages: *maxPages,
        ToCrawl:  toCrawl,
        ToSkip:   toSkip,
        Done:     done,
    }

    // Start the crawl manager
    go c.Start()

    // Signal handler to gracefully shutdown the crawl manager
    signalChan := make(chan os.Signal, 1)
    signal.Notify(signalChan, os.Interrupt)
    go func() {
        <-signalChan
        log.Println("Received interrupt signal, shutting down...")
        c.Stop()
    }()

    // Wait for all URLs to be crawled
    <-done

    log.Println("All URLs crawled successfully")
}

func download(u string, destDir string) error {
    // Create destination directory if it doesn't exist
    if _, err := os.Stat(destDir); os.IsNotExist(err) {
        if err := os.MkdirAll(destDir, 0755); err != nil {
            return fmt.Errorf("failed to create destination directory: %v", err)
        }
    }

    // Construct filename from URL
    fileName := u[strings.LastIndex(url, "/")+1:]

    // Download content from URL
    resp, err := http.Get(u)
    if err != nil {
        return fmt.Errorf("failed to get URL: %v", err)
    }
    defer resp.Body.Close()

    // Save content to file
    filePath := filepath.Join(destDir, fileName)
    file, err := os.Create(filePath)
    if err != nil {
        return fmt.Errorf("failed to create file: %v", err)
    }
    defer file.Close()

    _, err = io.Copy(file, resp.Body)
    if err != nil {
        return fmt.Errorf("failed to save content to file: %v", err)
    }

    return nil
}

func getHrefs(reader io.Reader) ([]string, error) {
    doc, err := html.Parse(reader)
    if err != nil {
        return nil, err
    }

    var hrefs []string
    var visitNode func(node *html.Node)
    visitNode = func(node *html.Node) {
        if node.Type == html.ElementNode && node.Data == "a" {
            for _, attr := range node.Attr {
                if attr.Key == "href" {
                    hrefs = append(hrefs, attr.Val)
                }
            }
        }
        for child := node.FirstChild; child != nil; child = child.NextSibling {
            visitNode(child)
        }
    }
    visitNode(doc)
    return hrefs, nil
}

func processURLs(urls []string, destDir string) {
    for _, url := range urls {
        resp, err := http.Get(url)
        if err != nil {
            log.Println("Error retrieving", url, ":", err)
            continue
        }
        defer resp.Body.Close()

        body, err := ioutil.ReadAll(resp.Body)
        if err != nil {
            log.Println("Error reading response from", url, ":", err)
            continue
        }

        hrefs := getHrefs(strings.NewReader(string(body)))
        for _, href := range hrefs {
            u, err := url.Parse(href)
            if err != nil {
                log.Println("Error parsing URL", href, ":", err)
                continue
            }
            if u.IsAbs() {
                processURLs([]string{u.String()}, destDir)
            } else {
                filename := filepath.Join(destDir, u.Path)
                err := ioutil.WriteFile(filename, body, 0644)
                if err != nil {
                    log.Println("Error writing file", filename, ":", err)
                    continue
                }
            }
        }
    }
}

func skipURL(url string, destination string) bool {
    _, err := os.Stat(destination + "/" + url2filename(url))
    if os.IsNotExist(err) {
        return false
    }
    return true
}

func resume(dest string) (map[string]bool, error) {
    downloaded := make(map[string]bool)

    // Read the downloaded URLs from the destination directory
    files, err := ioutil.ReadDir(dest)
    if err != nil {
        return nil, fmt.Errorf("failed to read directory: %v", err)
    }

    // Loop through each file and add its URL to the downloaded map
    for _, file := range files {
        url := file.Name()
        downloaded[url] = true
    }

    return downloaded, nil
}

// interruptHandler is a function that handles the interrupt signal
func interruptHandler(c chan os.Signal, done chan bool) {
    // wait for the signal
    <-c
    // notify that we are done
    done <- true
}

