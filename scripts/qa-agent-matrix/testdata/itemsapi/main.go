package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
)

func main() {
	var mu sync.Mutex
	var items []string
	http.HandleFunc("/items", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			items = append(items, strings.ToUpper(strings.TrimSpace(string(body))))
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			fmt.Fprint(w, strings.Join(items, ","))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	_ = http.ListenAndServe("127.0.0.1:"+os.Getenv("PORT"), nil)
}
