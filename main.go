package main

import (
	"fmt"
	"github.com/algorand/go-algorand-sdk/crypto"
	"github.com/algorand/go-algorand-sdk/mnemonic"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func printHelp() {
	help := `
Usage: galvanity [search-type] <pattern>
search-type is matching function to search for pattern, it can be:
  exact    - 0: search exact pattern (full address string)
  starts   - 1: search address which starts with given pattern
  ends     - 2: search address which ends with given pattern
  contains - 3: search address which contains given pattern at any place

Go to https://github.com/shmutalov/goalvanity to download the latest source code`

	fmt.Println(help)
}

const (
	searchTypeExact = iota
	searchTypeStartsWith
	searchTypeEndsWith
	searchTypeContains
)

var searchTypes = map[string]int{
	"exact":    searchTypeExact,
	"starts":   searchTypeStartsWith,
	"ends":     searchTypeEndsWith,
	"contains": searchTypeContains,
}

var searchTypeStrings = map[int]string{
	searchTypeExact:      "exact",
	searchTypeStartsWith: "starts",
	searchTypeEndsWith:   "ends",
	searchTypeContains:   "contains",
}

func verifyPattern(pattern string) bool {
	if ok, err := regexp.MatchString("^[A-Z2-7]+$", pattern); err == nil {
		return ok
	} else {
		return false
	}
}

func processArgs() (string, int, bool) {
	if len(os.Args) == 1 {
		fmt.Println("Not enough arguments")
		return "", 0, false
	}

	if len(os.Args) == 2 {
		return os.Args[1], searchTypeContains, true
	}

	// verify pattern that matches to base32 format
	pattern := strings.ToUpper(os.Args[2])
	if ok := verifyPattern(pattern); !ok {
		fmt.Println("Invalid pattern, must match to the BASE32 format: only A-Z and 2-7 (excluding 0/zero, 1/one, 8/eight and 9/nine)")
		return "", 0, false
	}

	if searchType, ok := searchTypes[strings.ToLower(os.Args[1])]; ok {
		return pattern, searchType, true
	} else {
		fmt.Println("Invalid search type")
		return "", 0, false
	}
}

func matchFunc(searchType int) (func(string, string) bool, bool) {
	switch searchType {
	case searchTypeExact:
		return func(address string, pattern string) bool {
			return address == pattern
		}, true
	case searchTypeStartsWith:
		return func(address string, pattern string) bool {
			return strings.HasPrefix(address, pattern)
		}, true
	case searchTypeEndsWith:
		return func(address string, pattern string) bool {
			return strings.HasSuffix(address, pattern)
		}, true
	case searchTypeContains:
		return func(address string, pattern string) bool {
			return strings.Contains(address, pattern)
		}, true
	default:
		fmt.Println("Unknown error, cannot determine a match-function")
		return nil, false
	}
}

func main() {
	pattern, searchType, ok := processArgs()
	if !ok {
		printHelp()
		return
	}

	matcherFn, ok := matchFunc(searchType)
	if !ok {
		printHelp()
		return
	}

	fmt.Printf("Pattern to find: %s\n", pattern)
	fmt.Printf("Search type: %s\n", searchTypeStrings[searchType])

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		os.Exit(1)
	}()

	fmt.Println("Matching started...")

	startedTime := time.Now()
	counter := make(chan uint64)
	for i := 0; i < runtime.NumCPU()-1; i++ {
		go func(pat string) {
			var i uint64
			for {
				account := crypto.GenerateAccount()
				if matcherFn(account.Address.String(), pat) {
					words, _ := mnemonic.FromPrivateKey(account.PrivateKey)
					fmt.Printf(
						`
==== ==== ====
Found ADDR: %s
PUB: %v
PK: %v
MNEMONIC: %s
==== ==== ====

`, account.Address, account.PublicKey, account.PrivateKey, words)
				}

				if i%100 == 0 {
					counter <- i
				}

				i++
				runtime.Gosched()
			}
		}(pattern)
	}

	var total uint64
	var oldTotal uint64
	oldTime := startedTime
	for {
		select {
		case x := <-counter:
			total += x
			if total%1_000_000 == 0 {
				if total == 0 {
					continue
				}

				now := time.Now()
				speed := (float64(total-oldTotal) / now.Sub(oldTime).Seconds()) / 1_000_000

				fmt.Printf("Processed: %d MH Speed: %.2f MH/s Time elapsed: %.2f s\n",
					total/1_000_000, speed, now.Sub(startedTime).Seconds())

				oldTotal = total
				oldTime = time.Now()
			}
		}

		runtime.Gosched()
	}
}
