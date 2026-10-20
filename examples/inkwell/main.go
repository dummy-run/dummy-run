// Inkwell is a small fictional publishing app. It exists so that Dummy Run personas
// have something safe to test: five roles, seeded test users, drafts, reviews,
// issues and payments. With -defects it plants faults named in a manifest file.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "address to listen on")
	defects := flag.String("defects", "", "manifest file that plants defects (none by default)")
	testLogin := flag.Bool("test-login", false, "enable /test-login?as=ROLE and /_reset for test builds")
	list := flag.Bool("list-switches", false, "print every switch a manifest can use and exit")
	flag.Parse()

	if *list {
		for _, n := range SwitchNames() {
			fmt.Printf("%-28s %s\n", n, switches[n])
		}
		return
	}
	var m *Manifest
	if *defects != "" {
		var err error
		if m, err = LoadManifest(*defects); err != nil {
			log.Fatal(err)
		}
	}
	s := NewServer(m, *testLogin)
	srv := &http.Server{
		Addr:              *addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	n := 0
	if m != nil {
		n = len(m.Defects)
	}
	log.Printf("inkwell on http://%s (defects: %d, test-login: %v)", *addr, n, *testLogin)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
