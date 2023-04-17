package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// structs
type WordStruct struct {
	MainWord        string          `json:"word"`
	Audio           string          `json:"audio"`
	Phonetic        string          `json:"phonetic"`
	PartsOfSpeeches []PartsOfSpeech `json:"parts_of_speeches"`
}

type PartsOfSpeech struct {
	PartsOfSpeech string       `json:"parts_of_speech"`
	Phonetic      string       `json:"phonetic"`
	Audio         string       `json:"audio"`
	Definitions   []Definition `json:"definitions"`
}

type Definition struct {
	Definition string   `json:"definition"`
	Example    string   `json:"example"`
	Synonyms   []string `json:"synonyms"`
	Antonyms   []string `json:"antonyms"`
}

type ErrorResponse struct {
	Message string `json:"message" xml:"message"`
}

// constants
const (
	mainContainer        = "#center_col"
	jsSlotsFilterTag     = `div[jsslot=""]`
	mainWordQueryTag     = `span[data-dobid="hdw"]`
	mainWordAudioTag     = "audio"
	mainWordPhoneticsTag = "span.LTKOO"
	posDivFilterTag      = `div[jsname="r5Nvmf"]`
	posPhoneticsTag      = "span.LTKOO"
	posAudioTag          = "audio"
	posTag               = "span.YrbPuc"
	posEachDefinitionTag = `[data-dobid="dfn"]`
	posSynAntParentTag   = `div[role="list"]`
)

// regex
var IsLetter = regexp.MustCompile(`^[a-zA-Z\s-]+$`).MatchString

const PORT = ":8080"
const ENV = "production"

var userAgents = []string{}

func main() {
	// get latest user agents
	getUserAgents()

	e := echo.New()
	e.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(100)))
	e.GET("/word/:word", func(c echo.Context) error {
		word := strings.ToLower(c.Param("word"))

		if !IsLetter(word) {
			return c.JSON(http.StatusUnprocessableEntity, &ErrorResponse{"Please provide word containing letters only."})
		}

		res, status := getContents(word)

		// fmt.Println(res, status)

		if status == http.StatusTooManyRequests {
			return c.JSON(http.StatusTooManyRequests, &ErrorResponse{"Too many request."})
		}

		if res.MainWord == "" {
			return c.JSON(http.StatusNotFound, &ErrorResponse{"No Definition found."})
		}

		return c.JSON(http.StatusOK, &res)
	})

	e.GET("/random", func(c echo.Context) error {

		return c.JSON(http.StatusOK, getRandomUserAgent())
	})

	e.GET("/ping", func(c echo.Context) error {
		return c.String(200,"hello there")
	})

	URL := ""

	if runtime.GOOS == "windows" {
		URL = "localhost" + PORT
	} else {
		URL = PORT
	}

	e.Logger.Fatal(e.Start(URL))

}

func getContents(word string) (*WordStruct, int) {

	defaultUserAgent := "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/110.0"
	randomUserAgent := getRandomUserAgent()
	userAgent := ""

	if len(userAgents) == 0 || randomUserAgent == "" {
		userAgent = defaultUserAgent
	} else {
		userAgent = randomUserAgent
	}

	fmt.Println(userAgent)

	// Instantiate default collector
	c := colly.NewCollector(
		colly.UserAgent(userAgent),
	)

	var wordS WordStruct
	errorStatus := 200

	// On every a element which has href attribute call callback
	c.OnHTML(mainContainer, func(e *colly.HTMLElement) {

		// fmt.Println(e.DOM.Html())

		// 	children 5 div with tag jsslot=""
		// 1. just the header
		// 2. search box
		// 3. all the definitions and the other things we want [our target]
		// 4. translations in other language
		// 5. use over time graph

		firstContainer := e.DOM.Find(".lr_container").First()

		// 3rd div with attribute jsslot="" go obtain the main data
		thirdJsSlot := firstContainer.Find(jsSlotsFilterTag).FilterFunction(func(i int, s *goquery.Selection) bool {
			return i == 2
		})

		// 	inside the 3rd div 4 dive
		// 1- the main word
		// 2- see definitions in
		// 3- meanings [our target]
		// 4- origin

		// find the main word
		mainWord := thirdJsSlot.Find(mainWordQueryTag).Text()
		fmt.Println("main word", mainWord)
		// check if it has phonetics and audio in the #1 div

		firstDiv := thirdJsSlot.Children().First()

		mainWordAudio := firstDiv.Find(mainWordAudioTag).Children().AttrOr("src", "")
		mainWordPhonetics := firstDiv.Find(mainWordPhoneticsTag).First().Text()

		fmt.Println(mainWordAudio, mainWordPhonetics)
		wordS.MainWord = mainWord
		wordS.Audio = mainWordAudio
		wordS.Phonetic = mainWordPhonetics

		child := thirdJsSlot.Children()

		allPoses := []PartsOfSpeech{}

		// inside #3 div
		child.Find(posDivFilterTag).Each(func(i int, s *goquery.Selection) {
			// we are inside each parts of speech div
			poses := PartsOfSpeech{}
			// fmt.Println("=========================================================================")
			// fmt.Println(i, "th div")
			// get the phonetics
			phonetics := s.Find(posPhoneticsTag).First().Text()
			// fmt.Println("phonetics ::", phonetics)
			// get pronunciation the audio source
			audio := s.Find(posAudioTag).Children().AttrOr("src", "")
			// fmt.Println("audio ::", audio)

			// get the parts of speech
			pos := s.Find(posTag).First().Text()
			// fmt.Println("pos ::", pos)

			poses.Phonetic = phonetics
			poses.Audio = audio
			poses.PartsOfSpeech = pos

			// each meanings with examples
			s.Find("ol > li").Children().Each(func(i int, s *goquery.Selection) {
				// definition
				dfnElement := s.Find(posEachDefinitionTag)
				definition := Definition{}

				dfn := dfnElement.Text()

				if dfn != "" {
					fmt.Println("definition ::", dfn)
					// get the example sentence
					exElement := dfnElement.Siblings()
					example := strings.Trim(exElement.First().Text(), "\"")

					// fmt.Println("example ::", example)

					// now lets find the synonym and antonyms
					var synonyms []string
					var antonyms []string

					currentType := "Similar"
					synAntElements := s.Find(posSynAntParentTag)

					synAntElements.Children().Each(func(i int, s *goquery.Selection) {

						txtToAdd := strings.TrimSpace(s.Text())
						chkIfToAdd := false

						// filter out the grayed out words from the results
						if s.Children().First().AttrOr("style", "") == "cursor:text" {
							chkIfToAdd = false
						} else {
							chkIfToAdd = true
						}

						// omit first div with text h
						// now encounter Similar or Opposite

						if txtToAdd == "Similar:" {
							currentType = "synonyms"
						}
						if txtToAdd == "Opposite:" {
							currentType = "antonyms"
						}

						if currentType == "synonyms" && txtToAdd != "Similar:" && txtToAdd != "h" && txtToAdd != "" && chkIfToAdd {
							synonyms = append(synonyms, txtToAdd)
						}
						if currentType == "antonyms" && txtToAdd != "Opposite:" && txtToAdd != "h" && txtToAdd != "" && chkIfToAdd {
							antonyms = append(antonyms, txtToAdd)
						}

					})

					definition.Definition = dfn
					definition.Example = example
					definition.Synonyms = synonyms
					definition.Antonyms = antonyms

					// fmt.Println(currentType)

					// fmt.Println("synonyms ::", strings.Join(synonyms, ","))
					// fmt.Println("antonyms ::", strings.Join(antonyms, ","))
					poses.Definitions = append(poses.Definitions, definition)

				}
			})
			allPoses = append(allPoses, poses)

			// fmt.Println("=========================================================================")

		})

		// fmt.Println("third div ===== 2",thirdJsSlot.First().Children().Size())
		// fmt.Println(thirdDiv.Html())

		wordS.PartsOfSpeeches = allPoses

		// fmt.Println("asdf",wordS)

	})

	// Before making a request print "Visiting ..."
	c.OnRequest(func(r *colly.Request) {
		fmt.Println("Visiting", r.URL.String())
	})

	// Set error handler
	c.OnError(func(r *colly.Response, err error) {
		errorStatus = r.StatusCode
		fmt.Println("Request URL:", r.Request.URL, "failed with response:", r, "\nError:", err)
		
	})

	// Start scraping on https://hackerspaces.org
	c.Visit("https://www.google.com/search?&hl=en&q=define+" + word)

	// fmt.Println(wordS)

	// b, err := json.MarshalIndent(wordS, "", "  ")
	// if err != nil {
	// 	fmt.Println(err)
	// }
	// fmt.Print(string(b))
	// fmt.Println("from here",wordS)
	return &wordS, errorStatus
}

func getUserAgents() {
	resp, err := http.Get("https://jnrbsn.github.io/user-agents/user-agents.json")
	if err != nil {
		log.Fatalln(err)
	}
	//We Read the response body on the line below.
	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		log.Fatalln(err)
	}
	//Convert the body to type string
	// sb := string(body)
	_ = json.Unmarshal(body, &userAgents)
	// log.Printf(sb)
	// log.Println(userAgents)

}

func getRandomUserAgent() string {
	s := rand.NewSource(time.Now().Unix())
	r := rand.New(s) // initialize local pseudorandom generator
	index := r.Intn(len(userAgents))
	return userAgents[index]
}
