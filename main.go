package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	schedule   = 15 * time.Minute
	postalCode = "0553"
)

var (
	//go:embed web/index.gohtml
	res   embed.FS
	pages = map[string]string{
		"/": "web/index.gohtml",
	}
	client          = http.DefaultClient
	currentResponse = new(PostalCodeLookupResponse)
)

type PostalCodeLookupResponse struct {
	StreetAddressRequest bool     `json:"isStreetAddressReq"`
	DeliveryDays         []string `json:"nextDeliveryDays"`
	LastUpdated          time.Time
}

func main() {
	kickstart := make(chan bool, 1)
	go fetchSchedule(kickstart)
	log.Println("kickstarting data fetching")
	kickstart <- true

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		time.Now()
		page, ok := pages[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		tpl, err := template.ParseFS(res, page)
		if err != nil {
			log.Printf("page %s not found in pages cache...", r.RequestURI)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)

		data := map[string]interface{}{
			"deliveryDays": currentResponse.DeliveryDays,
			"lastUpdated":  currentResponse.LastUpdated,
			"missingData":  len(currentResponse.DeliveryDays) == 0,
			"postalCode":   postalCode,
		}

		if err := tpl.Execute(w, data); err != nil {
			return
		}
	})
	http.FileServer(http.FS(res))
	port := os.Getenv("PORT")
	if port == "" {
		port = "5055"
	}
	log.Printf("server started on port %s...", port)
	err := http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
	if err != nil {
		panic(err)
	}
}

func fetchSchedule(externalEvent chan bool) {
	ticker := time.NewTicker(schedule)
	log.Printf("scheduling updates at interval %+v", schedule)

	for {
		select {
		case <-externalEvent:
			if data, err := fetchData(); err == nil {
				currentResponse = data
			}
		case <-ticker.C:
			if data, err := fetchData(); err == nil {
				currentResponse = data
			}
		}
	}
}

func fetchData() (*PostalCodeLookupResponse, error) {
	log.Println("fetching newest data from Posten")
	var p PostalCodeLookupResponse

	req, err := http.NewRequest("GET", fmt.Sprintf("https://www.posten.no/levering-av-post-2020/_/component/main/1/leftRegion/1?postCode=%s", postalCode), nil)
	if err != nil {
		log.Fatal("could not construct request, something is seriously wrong")
	}

	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	res, err := client.Do(req)
	if err != nil {
		log.Printf("could not fetch data from Posten: %+v", err)
		return nil, err
	}

	err = json.NewDecoder(res.Body).Decode(&p)
	if err != nil {
		log.Printf("could not decode JSON from Posten: %+v", err)
		return nil, err
	}

	log.Println("successfully fetched new data from Posten")
	p.LastUpdated = time.Now()
	return &p, nil
}
