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
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

/*
WARNING: episodes 67-123 seem to be missing from webpage. !!
*/

const seedUrl = "http://isaacmeyer.net/2013/03/timeline-and-glossary/"
const coverUrl = "http://isaacmeyer.net/wp-content/uploads/2017/11/cropped-Untitled-design.jpg"
const workingFolder = "/Users/halmohssen/src/historyOfJapan/data/"
const outFolder = workingFolder + "out/"
const mapFileName = workingFolder + "epMap"

//
type Episode struct {
	Mp3Url      string
	Mp3LenSecs  int
	Url         string
	Num         int
	Description string
	Images      []Image
}

func (ep Episode) htmlFileName() string {
	return workingFolder + fmt.Sprintf("%d.html", ep.Num)
}

type Image struct {
	RawUrl  string
	Caption string
}

func (img Image) clearnedUrl() string {
	return strings.Split(img.RawUrl, "?")[0]
}

func (img Image) cacheFile() string {
	return cacheFile(img.clearnedUrl())
}

func cacheFile(clearnedURL string) string {
	b := strings.Split(clearnedURL, ".")
	ext := "." + b[len(b)-1]
	return workingFolder + hash(clearnedURL) + ext
}
func main() {
	var epMap map[int]Episode //episode # -> full data of EP.

	if !fileThere(mapFileName) {
		walkAndSaveEpMapSkel()
	}

	epMap = loadEpMap()
	fmt.Println("loaded epMap, size is %d", len(epMap))

	cacheResource(coverUrl, cacheFile(coverUrl)) //shared for all episodes

	for num, ep := range epMap {
		cacheEpFileName := ep.htmlFileName()
		if !fileThere(cacheEpFileName) {
			cacheResource(ep.Url, cacheEpFileName)
		}
		ep.processEpDataFromLocalCache()
		ep.writeVideoScript()
		epMap[num] = ep //golang does not support change in place
	}

}

func walkAndSaveEpMapSkel() {
	fmt.Println("WARNING: finding URLs for all episodes")
	var epMap = make(map[int]Episode)
	var epNum = 1

	// Instantiate default collector
	c := colly.NewCollector(
		// MaxDepth is 2, so only the links on the scraped page
		// and links on those pages are visited
		colly.MaxDepth(300),
		colly.Async(true),
	)

	// Limit the maximum parallelism to 2
	// This is necessary if the goroutines are dynamically
	// created to control the limit of simultaneous requests.
	//
	// Parallelism can be controlled also by spawning fixed
	// number of go routines.
	assertNoErr(c.Limit(&colly.LimitRule{DomainGlob: "*", Parallelism: 2}), "c.limit")

	// On every a element which has href attribute call callback
	c.OnHTML(".post-navigation-inner .post-nav-next a[title^=Next]", func(e *colly.HTMLElement) {
		nextEpUrl := e.Attr("href")
		newEp := Episode{Num: epNum, Url: nextEpUrl, Description: "PLACE_HOLDER", Images: make([]Image, 0, 5)}
		epMap[epNum] = newEp
		epNum++
		// Print nextEpUrl
		fmt.Printf("\tep=%+v\n", newEp)
		// Visit nextEpUrl found on page on a new thread
		assertNoErr(e.Request.Visit(nextEpUrl), "on appending a visit")
	})

	// Start scraping on last blog post
	assertNoErr(c.Visit(seedUrl), "visit error")
	// Wait until threads are finished
	c.Wait()
	bolB, _ := json.Marshal(epMap)
	check(ioutil.WriteFile(mapFileName, bolB, 0644))

}

func (ep *Episode) processEpDataFromLocalCache() {
	localFilename := ep.htmlFileName()
	//setup the html parsign engine; according to the colly example I have to do the next few lines for the magic to work
	t := &http.Transport{}
	t.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
	c := colly.NewCollector()
	c.WithTransport(t)

	localUrl := "file://" + localFilename
	fmt.Printf("parsing %q...{\t", localUrl)

	//  for every post download the Images and the captions:
	c.OnHTML(".post-content [id^=attachment_]", func(e *colly.HTMLElement) {
		var imgSrc = e.ChildAttr("img", "src")
		//var txtCaption = e.ChildAttr("img", "alt")
		txtCaption := e.ChildText(".wp-caption-text")
		img := Image{RawUrl: imgSrc, Caption: txtCaption}
		go cacheResource(img.clearnedUrl(), img.cacheFile())
		// Visit link found on page on a new thread
		ep.addImg(img)
		//fmt.Printf("debugging length = %d raw =%+v",len(ep.Images),ep.Images)
	})

	// for every post download the audio:
	c.OnHTML(".post-content [href$=mp3]", func(e *colly.HTMLElement) {
		mp3Url := e.Attr("href")
		ep.Mp3Url = mp3Url
		cacheResource(ep.Mp3Url, ep.mp3LocalCache())
		ep.Mp3LenSecs = lenOfMp3InSecs(ep.mp3LocalCache())
	})

	// Start scraping on last blog post
	assertNoErr(c.Visit(localUrl), "an other visit error")
	// Wait until threads are finished
	c.Wait()
	fmt.Println("\n\t} done")
}

func (ep Episode) mp3LocalCache() string {
	return cacheFile(ep.Mp3Url)
}

func (ep *Episode) addImg(img Image) []Image {
	ep.Images = append(ep.Images, img) //?what odd syntax ! https://stackoverflow.com/questions/18042439/go-append-to-slice-in-struct
	return ep.Images
}

// writes a script that if run will:
//0. puts in a cover page
//1. generate the caption images

//-2. ends with a cover page
//-1. generates an MP4 with the audio and images
func (ep *Episode) writeVideoScript() {
	covFile := cacheFile(coverUrl)
	script := ""
	script = script + fmt.Sprintf("#!/bin/bash\n") +
		fmt.Sprintf("#\n") +
		fmt.Sprintf("# MACHINE GENERATED SCRIPT DO NOT EDIT !\n") +
		fmt.Sprintf("# THIS SCRIPT IS MEANT TO GENERATE A VIDEO FROM THE WEBPAGE %q\n", ep.Url) +
		fmt.Sprintf("# \t the mp3 file %q\t is %d seconds long \n \t \t it is stored in %q \n", ep.Mp3Url, ep.Mp3LenSecs, ep.mp3LocalCache()) +
		fmt.Sprintf("# coverFile %q\n", covFile) +
		fmt.Sprintf("\n") +
		fmt.Sprintf("\n") +
		fmt.Sprintf("# mp3 file = %q\n", ep.mp3LocalCache())
	for _, img := range ep.Images {
		script += fmt.Sprintf("# img= %q\n", img.cacheFile())
	}

	epOutFolder := outFolder + strconv.Itoa(ep.Num) //+"/"
	os.RemoveAll(epOutFolder)
	assertNoErr(os.MkdirAll(epOutFolder, 0755), "can't create a folder")
	assertNoErr(ioutil.WriteFile(epOutFolder+"/run.sh", []byte(script), 0755), "could not write a run.sh file!")
}

func loadEpMap() map[int]Episode {
	urlMap := make(map[int]Episode)
	fileBytes, readError := ioutil.ReadFile(mapFileName)
	assertNoErr(readError, "could not read episode Map file")

	unmartialError := json.Unmarshal(fileBytes, &urlMap)
	assertNoErr(unmartialError, "could not parse Map file")
	return urlMap
}

func fileThere(filename string) bool {
	handle, err := os.Stat(filename)
	if err != nil {
		if os.IsExist(err) {
			return true
		} else {
			return false
		}
	}
	if handle.Size() > 0 {
		return true
	} else {
		return false
	}
}

//only download a resource (image, file etc.) if the local cache does not exist
//warning: does not check for validity of local cache, partial downloads will trick this function
func cacheResource(url string, localCache string) {
	if fileThere(localCache) {
		//fmt.Println("debug: found ", outputFileName)
		return
	}
	fmt.Printf("%q NOT cached 😅, downloading to %q ... ", url, localCache)
	// don't worry about errors
	response, e := http.Get(url)
	assertNoErr(e, "could not http.get "+url)
	defer response.Body.Close()
	//open a file for writing
	file, err := os.Create(localCache)
	check(err)
	// Use io.Copy to just dump the response body to the file. This supports huge files
	_, err = io.Copy(file, response.Body)
	check(err)
	fmt.Println("... done 🤗")
}

func hash(text string) string {
	algorithm := fnv.New32a()
	_, err := algorithm.Write([]byte(text))
	assertNoErr(err, "hashing problem")
	return fmt.Sprintf("%v", algorithm.Sum32())
}

func assertNoErr(err error, msg string) {
	if err != nil {
		fmt.Printf("\nERROR! \n \t msg=%q \n err=%q\n\t stack trace=%+v", msg, err, runtime.StartTrace())
	}
}

func check(e error) {
	if e != nil {
		fmt.Println("wow we have an error! ")
		fmt.Println(runtime.StartTrace())
		panic(e)
	}
}

func lenOfMp3InSecs(mp3Filename string) int {
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", mp3Filename).Output()
	assertNoErr(err, "problem execing ffprobe, not installed? ")
	lenF, err := strconv.ParseFloat(strings.Trim(string(out), "\n"), 32)
	assertNoErr(err, "could not parse return of ffprobe! for file !"+mp3Filename)
	return int(lenF)
}
