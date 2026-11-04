package main

import (
	"fmt"
	"log"
	"net/http"
)

// welcome writes a small HTML page that greets the visitor
func welcome(w http.ResponseWriter, title string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>%s</title>
</head>
<body>
  <h1>Hi, welcome to the %s page!</h1>
</body>
</html>`, title, title)
}

func homePage(w http.ResponseWriter, r *http.Request) {
	welcome(w, "Home")
}

func coursePage(w http.ResponseWriter, r *http.Request) {
	welcome(w, "Courses")
}

func aboutPage(w http.ResponseWriter, r *http.Request) {
	welcome(w, "About")
}

func contactPage(w http.ResponseWriter, r *http.Request) {
	welcome(w, "Contact")
}

func main() {

	http.HandleFunc("/home", homePage)
	http.HandleFunc("/courses", coursePage)
	http.HandleFunc("/about", aboutPage)
	http.HandleFunc("/contact", contactPage)

	err := http.ListenAndServe("0.0.0.0:8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}
