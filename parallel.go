package main

import (
	"encoding/json"
	"fmt"
	"github.com/gocolly/colly"
	"hash/fnv"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"runtime"
	"strings"
)

const startUrl = "http://isaacmeyer.net/2018/12/episode-269-the-revolution-will-not-be-live/"
const episodeNum = 269
const workingFolder = "/Users/halmohssen/src/historyOfJapan/data/"
const mapFileName = workingFolder + "urlMap"

func main() {
	if !fileThere(mapFileName) {
		downloadAndSavePostUrls()
	}

	urlMap := loadPostUrls()
	fmt.Printf("loaded urlMap, size is %d", len(urlMap))

	for ep, epUrl := range urlMap {
		cacheEpFileName := localEpHtmlFNameFromEpNum(ep)
		if !fileThere(cacheEpFileName) {
			downloadURLandSaveLocally(epUrl, cacheEpFileName)
		}
		processEpDataFromLocalCache(cacheEpFileName)
	}
}

func check(e error) {
	if e != nil {
		fmt.Println("wow we have an error! ")
		fmt.Println(runtime.StartTrace())
		panic(e)
	}
}

func downloadAndSavePostUrls() {
	fmt.Println("WARNING: finding URLs for all episodes")
	var urlMap = make(map[int]string)
	var episodeKey = episodeNum

	// Instantiate default collector
	c := colly.NewCollector(
		// MaxDepth is 2, so only the links on the scraped page
		// and links on those pages are visited
		colly.MaxDepth(268),
		colly.Async(true),
	)

	// Limit the maximum parallelism to 2
	// This is necessary if the goroutines are dynamically
	// created to control the limit of simultaneous requests.
	//
	// Parallelism can be controlled also by spawning fixed
	// number of go routines.
	c.Limit(&colly.LimitRule{DomainGlob: "*", Parallelism: 2})

	// On every a element which has href attribute call callback
	c.OnHTML(".post-navigation-inner .post-nav-prev a[href]", func(e *colly.HTMLElement) {
		link := e.Attr("href")
		urlMap[episodeKey] = link
		episodeKey--
		// Print link
		fmt.Printf("key=%d,\t %q\n ", episodeKey, link)
		// Visit link found on page on a new thread
		e.Request.Visit(link)
	})

	// Start scraping on last blog post
	c.Visit(startUrl)
	// Wait until threads are finished
	c.Wait()

	bolB, _ := json.Marshal(urlMap)

	check(ioutil.WriteFile(mapFileName, bolB, 0644))

}

func loadPostUrls() map[int]string {
	urlMap := make(map[int]string)
	fileBytes, readError := ioutil.ReadFile(mapFileName)
	check(readError)

	unmartialError := json.Unmarshal(fileBytes, &urlMap)
	check(unmartialError)
	return urlMap
}

func processEpDataFromLocalCache(localFilename string) {
	//setup the html parsign engine; according to the colly example I have to do the next few lines for the magic to work
	t := &http.Transport{}
	t.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
	c := colly.NewCollector()
	c.WithTransport(t)

	localUrl := "file://" + localFilename
	fmt.Printf("parsing file %q...\n", localUrl)

	// On every a element which has href attribute call callback
	c.OnHTML(".post-content [id^=attachment_]", func(e *colly.HTMLElement) {
		var imgSrc = e.ChildAttr("img", "src")
		var txtCaption = e.ChildAttr("img", "alt")
		//fmt.Printf("localFilename=%q img[src]=%q, caption=%q\n", localFilename, imgSrc, txtCaption)
		downloadImageIfNotCached(imgSrc)
		saveTxtToImage(imgSrc, txtCaption)
		// Visit link found on page on a new thread
	})

	// Start scraping on last blog post
	c.Visit(localUrl)
	// Wait until threads are finished
	c.Wait()
}

func fileThere(filename string) bool {
	a, err := os.Stat(filename)
	if err != nil {
		if !os.IsNotExist(err) {
			return true
		}
	}
	if a.Size() > 0 {
		return true
	}
	return false
}

func saveTxtToImage(url string, txt string) {
	_, outputFileName := cleanUrlAndLocalHasFilename(url)
	txtFileName := outputFileName + ".txt"
	if fileThere(txtFileName) {
		return
	} else {
		fmt.Printf("creating caption file %q", txtFileName)
		ioutil.WriteFile(txtFileName, []byte(txt), 0644)
	}
}

func cleanUrlAndLocalHasFilename(url string) (string, string) {
	clearnedUrl := strings.Split(url, "?")[0]
	b := strings.Split(clearnedUrl, ".")
	ext := "." + b[len(b)-1]
	localFileName := workingFolder + hash(clearnedUrl) + ext
	return clearnedUrl, localFileName
}

func downloadImageIfNotCached(url string) {
	clearnedUrl, outputFileName := cleanUrlAndLocalHasFilename(url)

	if fileThere(outputFileName) {
		//fmt.Println("debug: found ", outputFileName)
		return
	}
	downloadURLandSaveLocally(clearnedUrl, outputFileName)
}

func downloadURLandSaveLocally(remoteCleanUrl string, localFilename string) {
	fmt.Printf("WARNING could NOT find %q, downloading to %q !!!", remoteCleanUrl, localFilename)
	// don't worry about errors
	response, e := http.Get(remoteCleanUrl)
	check(e)
	defer response.Body.Close()
	//open a file for writing
	file, err := os.Create(localFilename)
	check(err)
	// Use io.Copy to just dump the response body to the file. This supports huge files
	_, err = io.Copy(file, response.Body)
	check(err)
	fmt.Println("downloaded " + remoteCleanUrl)
}

func localEpHtmlFNameFromEpNum(i int) string {
	return workingFolder + fmt.Sprintf("%d.html", i)
}

func hash(text string) string {
	algorithm := fnv.New32a()
	algorithm.Write([]byte(text))
	return fmt.Sprintf("%v", algorithm.Sum32())
}
