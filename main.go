package main

import (
	"log"
	"net/http"
)

func landingPage(w http.ResponseWriter, r *http.Request) {
	// Render the home html page from static folder
	http.ServeFile(w, r, "static/landing.html")
}

func main() {

	http.HandleFunc("/", landingpage)

	err := http.ListenAndServe("0.0.0.0:8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}
